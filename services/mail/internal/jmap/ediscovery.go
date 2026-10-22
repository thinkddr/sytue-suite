// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// API de administración de compliance: legal hold + eDiscovery (paridad M365
// Purview / Google Vault, la pieza NO destructiva). Preservar correo para
// litigios y permitir búsqueda + export controlada por un admin/owner de la org.
//
// ACCESO FÉRREO: esto es acceso privilegiado a correo AJENO. Doble puerta:
//  1. Middleware verifier.RequireRole(owner|admin) en el montaje (Mount).
//  2. requireAdmin en cada handler (defensa en profundidad; también hace el
//     403 unit-testeable sin el middleware).
//
// AISLAMIENTO POR ORG: todo corre bajo el tenant del token (RLS). Un admin de la
// org A jamás resuelve ni busca correo/holds de la org B (AccountBy* devuelve
// ErrNotFound para sujetos de otra org).
// AUDITORÍA: cada crear/liberar hold y cada búsqueda/export se registra en
// audit_log (quién, qué sujeto, qué caso). La búsqueda/export auditan en modo
// fail-closed: si la traza no se escribe, NO se sirve el resultado.
package jmap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// complianceRoles son los roles autorizados a legal hold + eDiscovery.
// ponytail: admin/owner hasta que aterricen los «roles personalizados»; entonces
// gatear por un permiso granular «compliance» en vez de por rol de org.
var complianceRoles = []string{"owner", "admin"}

// complianceStore es la superficie de persistencia de compliance. La satisface
// *store.Store (producción) y un fake en tests (sin PostgreSQL).
type complianceStore interface {
	AccountBySubject(ctx context.Context, subject string) (domain.Account, error)
	AccountByAddress(ctx context.Context, address string) (domain.Account, error)
	CreateHold(ctx context.Context, h domain.LegalHold) (domain.LegalHold, error)
	ListHolds(ctx context.Context, includeReleased bool) ([]domain.LegalHold, error)
	ReleaseHold(ctx context.Context, id uuid.UUID, actor string) error
	EDiscoverySearch(ctx context.Context, accountID uuid.UUID, f store.EDiscoveryFilter) ([]domain.EDiscoveryHit, error)
	WriteAudit(ctx context.Context, actor, action, target string, detail any) error
	ListAudit(ctx context.Context, limit int) ([]domain.AuditEntry, error)
}

// requireAdmin exige claims con rol de compliance (owner|admin). Defensa en
// profundidad tras el middleware RequireRole del montaje.
func requireAdmin(r *http.Request) (authn.Claims, error) {
	claims, ok := authn.FromContext(r.Context())
	if !ok {
		return authn.Claims{}, httpx.ErrUnauthorized
	}
	if !claims.HasAnyRole(complianceRoles...) {
		return authn.Claims{}, httpx.ErrForbidden.WithMessage("Requires the administrator or owner role of the organisation.")
	}
	return claims, nil
}

// --- Legal holds ----------------------------------------------------------

type createHoldReq struct {
	Email   string `json:"email,omitempty"`   // usuario objetivo por dirección
	Subject string `json:"subject,omitempty"` // o por sub de Sytue ID
	OrgWide bool   `json:"org_wide,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Case    string `json:"case,omitempty"`
}

func (a *API) createHold(w http.ResponseWriter, r *http.Request) {
	claims, err := requireAdmin(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body createHoldReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	subject := ""
	if !body.OrgWide {
		// Resolver el sujeto a una cuenta REAL de esta org (aislamiento): un email/
		// sub de otra org devuelve ErrNotFound => 404, no se crea el hold.
		acc, err := a.resolveTarget(r, body.Email, body.Subject)
		if err != nil {
			httpx.Fail(w, r, mapErr(err))
			return
		}
		subject = acc.Subject
	}

	hold, err := a.compliance.CreateHold(r.Context(), domain.LegalHold{
		Subject:   subject,
		Reason:    body.Reason,
		Case:      body.Case,
		CreatedBy: claims.Subject,
	})
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, hold)
}

func (a *API) listHolds(w http.ResponseWriter, r *http.Request) {
	if _, err := requireAdmin(r); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	holds, err := a.compliance.ListHolds(r.Context(), r.URL.Query().Get("include_released") == "true")
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"holds": holds})
}

func (a *API) releaseHold(w http.ResponseWriter, r *http.Request) {
	claims, err := requireAdmin(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "holdID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid holdID."))
		return
	}
	if err := a.compliance.ReleaseHold(r.Context(), id, claims.Subject); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

// --- eDiscovery -----------------------------------------------------------

type ediscoverySearchReq struct {
	Email        string `json:"email,omitempty"`   // usuario objetivo por dirección
	Subject      string `json:"subject,omitempty"` // o por sub de Sytue ID
	From         string `json:"from,omitempty"`    // fecha desde (RFC3339 o YYYY-MM-DD)
	To           string `json:"to,omitempty"`      // fecha hasta (exclusiva)
	Query        string `json:"query,omitempty"`   // texto libre (asunto/remitente/cuerpo)
	FromAddr     string `json:"from_addr,omitempty"`
	SubjectQuery string `json:"subject_query,omitempty"`
	Case         string `json:"case,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Offset       int    `json:"offset,omitempty"`
}

// ediscoveryMeta es el metadato de un mensaje en los resultados de búsqueda: SIN
// cuerpo completo (el export baja el .eml). Marca Preserved el correo retenido.
type ediscoveryMeta struct {
	ID         uuid.UUID `json:"id"`
	ThreadID   uuid.UUID `json:"thread_id"`
	MessageID  string    `json:"message_id,omitempty"`
	Subject    string    `json:"subject"`
	From       string    `json:"from"`
	FromName   string    `json:"from_name,omitempty"`
	To         []string  `json:"to,omitempty"`
	Cc         []string  `json:"cc,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
	Size       int64     `json:"size"`
	HasAttach  bool      `json:"has_attachment"`
	MailboxID  uuid.UUID `json:"mailbox_id"`
	Preserved  bool      `json:"preserved"`
	Snippet    string    `json:"preview,omitempty"`
}

func (a *API) ediscoverySearch(w http.ResponseWriter, r *http.Request) {
	claims, err := requireAdmin(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body ediscoverySearchReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	acc, err := a.resolveTarget(r, body.Email, body.Subject)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	from, to, err := parseRange(body.From, body.To)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	hits, err := a.compliance.EDiscoverySearch(r.Context(), acc.ID, store.EDiscoveryFilter{
		From: from, To: to, Query: body.Query, FromAddr: body.FromAddr,
		Subject: body.SubjectQuery, Limit: body.Limit, Offset: body.Offset,
	})
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}

	// Fail-closed: sin traza no se sirve el acceso privilegiado.
	if err := a.auditAccess(r, claims.Subject, "ediscovery.search", acc, body.Case, map[string]any{
		"query": body.Query, "from": body.From, "to": body.To,
		"from_addr": body.FromAddr, "subject_query": body.SubjectQuery,
		"target_subject": acc.Subject, "results": len(hits),
	}); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	out := make([]ediscoveryMeta, 0, len(hits))
	for _, h := range hits {
		m := h.Message
		out = append(out, ediscoveryMeta{
			ID: m.ID, ThreadID: m.ThreadID, MessageID: m.MessageID, Subject: m.Subject,
			From: m.FromAddr, FromName: m.FromName, To: m.To, Cc: m.Cc,
			ReceivedAt: m.ReceivedAt, Size: m.SizeBytes, HasAttach: m.HasAttach,
			MailboxID: m.MailboxID, Preserved: h.Preserved, Snippet: m.Snippet,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"target":   map[string]any{"email": acc.Address, "subject": acc.Subject},
		"total":    len(out),
		"messages": out,
	})
}

// ediscoveryExport streamea el conjunto de mensajes del caso como un .mbox
// (formato de export estándar de Vault/Takeout; una sola pasada, sin buffer).
// GET /api/v1/admin/ediscovery/export?email=..&from=..&to=..&query=..&case=..
func (a *API) ediscoveryExport(w http.ResponseWriter, r *http.Request) {
	claims, err := requireAdmin(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := r.URL.Query()
	acc, err := a.resolveTarget(r, q.Get("email"), q.Get("subject"))
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	from, to, err := parseRange(q.Get("from"), q.Get("to"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if a.blobs == nil {
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("Object storage is unavailable; the export cannot be produced."))
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}

	hits, err := a.compliance.EDiscoverySearch(r.Context(), acc.ID, store.EDiscoveryFilter{
		From: from, To: to, Query: q.Get("query"), FromAddr: q.Get("from_addr"),
		Subject: q.Get("subject_query"), Limit: 5000,
	})
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}

	// Fail-closed ANTES de escribir la cabecera 200 (una vez empezado el stream ya
	// no se puede cambiar el status).
	if err := a.auditAccess(r, claims.Subject, "ediscovery.export", acc, q.Get("case"), map[string]any{
		"query": q.Get("query"), "from": q.Get("from"), "to": q.Get("to"),
		"target_subject": acc.Subject, "results": len(hits), "format": "mbox",
	}); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/mbox")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q",
		"ediscovery-"+sanitizeFilename(acc.Address)+"-"+time.Now().UTC().Format("20060102")+".mbox"))
	w.WriteHeader(http.StatusOK)

	for _, h := range hits {
		blob, err := a.blobs.Get(r.Context(), t, h.Message.BlobKey)
		if err != nil {
			// Blob ausente (p. ej. reapado por GC externo): se anota y se continúa;
			// no aborta el export (el status 200 ya se envió).
			a.logger.Warn("ediscovery export: blob no disponible", "message_id", h.Message.ID, "error", err)
			continue
		}
		raw, err := io.ReadAll(blob)
		_ = blob.Close()
		if err != nil {
			a.logger.Warn("ediscovery export: fallo al leer blob", "message_id", h.Message.ID, "error", err)
			continue
		}
		if err := writeMboxEntry(w, h.Message.FromAddr, h.Message.ReceivedAt, raw); err != nil {
			a.logger.Warn("ediscovery export: fallo al escribir mbox", "error", err)
			return
		}
	}
}

// --- helpers --------------------------------------------------------------

func (a *API) resolveTarget(r *http.Request, email, subject string) (domain.Account, error) {
	switch {
	case email != "":
		return a.compliance.AccountByAddress(r.Context(), email)
	case subject != "":
		return a.compliance.AccountBySubject(r.Context(), subject)
	default:
		return domain.Account{}, httpx.ErrBadRequest.WithMessage("Indica email o subject del usuario objetivo.")
	}
}

// auditAccess escribe la traza del acceso privilegiado. Devuelve ErrInternal si
// falla (el llamador es fail-closed: no sirve el resultado sin traza).
func (a *API) auditAccess(r *http.Request, actor, action string, target domain.Account, caseRef string, detail map[string]any) error {
	detail["case"] = caseRef
	if err := a.compliance.WriteAudit(r.Context(), actor, action, target.Address, detail); err != nil {
		a.logger.Error("compliance: fallo al auditar acceso privilegiado", "action", action, "error", err)
		return httpx.ErrInternal.WithMessage("The audit entry could not be written; the action was aborted.").Wrap(err)
	}
	return nil
}

// parseRange parsea el rango de fechas (RFC3339 o YYYY-MM-DD). Vacío = sin límite.
func parseRange(fromStr, toStr string) (from, to time.Time, err error) {
	if from, err = auditlog.LeerFecha(fromStr); err != nil {
		return from, to, err
	}
	to, err = auditlog.LeerFecha(toStr)
	return from, to, err
}

// sanitizeFilename deja un nombre de fichero seguro (sin separadores ni control).
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '/' || r == '\\' || r == '"' || r < 0x20 {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// writeMboxEntry escribe un mensaje en formato mboxrd: línea separadora "From "
// (asctime UTC) + el .eml crudo con las líneas ">*From " escapadas con un ">"
// extra (para no confundirlas con el separador) + línea en blanco de cierre.
func writeMboxEntry(w io.Writer, sender string, received time.Time, raw []byte) error {
	if sender == "" {
		sender = "MAILER-DAEMON"
	}
	if _, err := fmt.Fprintf(w, "From %s %s\n", sender, received.UTC().Format(time.ANSIC)); err != nil {
		return err
	}
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i < len(raw) && raw[i] != '\n' {
			continue
		}
		var line []byte
		if i < len(raw) {
			line = raw[start : i+1] // incluye el \n
		} else {
			line = raw[start:i] // última línea sin \n
		}
		content := bytes.TrimRight(line, "\r\n")
		if mboxNeedsQuote(content) {
			if _, err := w.Write([]byte{'>'}); err != nil {
				return err
			}
		}
		if _, err := w.Write(line); err != nil {
			return err
		}
		start = i + 1
	}
	// Asegurar salto tras el cuerpo + línea en blanco antes del próximo "From ".
	tail := "\n\n"
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		tail = "\n"
	}
	_, err := io.WriteString(w, tail)
	return err
}

// mboxNeedsQuote indica si una línea (sin CRLF) es "From " precedida de cero o
// más '>' (mboxrd): esas se escapan con un '>' extra.
func mboxNeedsQuote(content []byte) bool {
	j := 0
	for j < len(content) && content[j] == '>' {
		j++
	}
	return bytes.HasPrefix(content[j:], []byte("From "))
}
