// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// API de administración de compliance de Sytue Drive: legal hold + eDiscovery
// (paridad M365 Purview / Google Vault, la pieza NO destructiva; espeja la ola de
// Mail). Preservar ficheros para litigios y permitir búsqueda + export controlada
// por un admin/owner de la org.
//
// ACCESO FÉRREO: esto es acceso privilegiado a ficheros AJENOS. Doble puerta:
//  1. Middleware verifier.RequireRole(owner|admin) en el montaje (server.go).
//  2. requireAdmin en cada handler (defensa en profundidad; hace el 403
//     unit-testeable sin el middleware).
//
// AISLAMIENTO POR ORG: todo corre bajo el tenant del token, con predicado
// `tenant = $n` EXPLÍCITO en el store (defensa en profundidad sobre RLS). Un admin
// de la org A jamás busca ni exporta ficheros/holds/traza de la org B.
//
// AUDITORÍA: crear/liberar hold se auditan atómicamente en su tx; búsqueda/export
// auditan en modo FAIL-CLOSED (si la traza no se escribe, NO se sirve el resultado).
package drivehttp

import (
	"archive/zip"
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
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// complianceRoles son los roles autorizados a legal hold + eDiscovery.
// ponytail: admin/owner hasta que aterricen los «roles personalizados»; entonces
// gatear por un permiso granular «compliance» en vez de por rol de org.
var complianceRoles = []string{"owner", "admin"}

// complianceStore es la superficie de persistencia de compliance. La satisface
// *store.Store (producción) y un fake en tests (sin PostgreSQL).
type complianceStore interface {
	CreateHold(ctx context.Context, h domain.LegalHold) (domain.LegalHold, error)
	ListHolds(ctx context.Context, includeReleased bool) ([]domain.LegalHold, error)
	ReleaseHold(ctx context.Context, id uuid.UUID, actor string) error
	EDiscoverySearch(ctx context.Context, subject string, f store.EDiscoveryFilter) ([]domain.EDiscoveryHit, error)
	WriteAudit(ctx context.Context, actor, action, target string, detail any) error
	ListAudit(ctx context.Context, limit int) ([]domain.AuditEntry, error)
}

// blobGetter es la superficie mínima del objectstore para el export (leer el blob
// de cada fichero de S3). La satisface *objectstore.Client.
type blobGetter interface {
	Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error)
}

// complianceHandlers agrupa los handlers admin de compliance. Desacoplado del
// Handlers de la app (y de su fake) para no ensuciar esa superficie ni sus tests.
type complianceHandlers struct {
	store complianceStore
	blobs blobGetter // nil en dev sin S3: el export responde 500 controlado
}

// requireAdmin exige claims con rol de compliance (owner|admin). Defensa en
// profundidad tras el middleware RequireRole del montaje.
func requireAdmin(r *http.Request) (authn.Claims, error) {
	claims, ok := authn.FromContext(r.Context())
	if !ok {
		return authn.Claims{}, httpx.ErrUnauthorized
	}
	if !claims.HasAnyRole(complianceRoles...) {
		return authn.Claims{}, httpx.ErrForbidden.WithMessage("This requires the administrator or owner role in the organisation.")
	}
	return claims, nil
}

// --- Legal holds ----------------------------------------------------------

type createHoldReq struct {
	Subject string `json:"subject,omitempty"` // sub de Sytue ID (owner_sub) del usuario objetivo
	OrgWide bool   `json:"org_wide,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Case    string `json:"case,omitempty"`
}

func (a *complianceHandlers) createHold(w http.ResponseWriter, r *http.Request) {
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
	subject := strings.TrimSpace(body.Subject)
	if !body.OrgWide && subject == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Give the subject of the target user, or set org_wide."))
		return
	}
	if body.OrgWide {
		subject = "" // hold de toda la org
	}
	hold, err := a.store.CreateHold(r.Context(), domain.LegalHold{
		Subject:   subject,
		Reason:    body.Reason,
		Case:      body.Case,
		CreatedBy: claims.Subject,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, hold)
}

func (a *complianceHandlers) listHolds(w http.ResponseWriter, r *http.Request) {
	if _, err := requireAdmin(r); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	holds, err := a.store.ListHolds(r.Context(), r.URL.Query().Get("include_released") == "true")
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"holds": holds})
}

func (a *complianceHandlers) releaseHold(w http.ResponseWriter, r *http.Request) {
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
	if err := a.store.ReleaseHold(r.Context(), id, claims.Subject); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// --- eDiscovery -----------------------------------------------------------

type ediscoverySearchReq struct {
	Subject     string `json:"subject"`                // usuario objetivo por sub (owner_sub)
	From        string `json:"from,omitempty"`         // fecha desde (RFC3339 o YYYY-MM-DD)
	To          string `json:"to,omitempty"`           // fecha hasta (exclusiva)
	Name        string `json:"name,omitempty"`         // subcadena del nombre
	ContentType string `json:"content_type,omitempty"` // tipo MIME exacto
	// LabelID acota a lo clasificado con ESA etiqueta de sensibilidad (uuid de la
	// etiqueta en Sytue ID). Casa contra el SNAPSHOT del fichero, así que sigue
	// encontrando lo etiquetado aunque la etiqueta ya no exista en la taxonomía.
	LabelID string `json:"label_id,omitempty"`
	Case    string `json:"case,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Offset  int    `json:"offset,omitempty"`
}

// ediscoveryMeta es el metadato de un fichero en los resultados de búsqueda: SIN
// contenido (el export baja los blobs). Marca Preserved el fichero retenido.
type ediscoveryMeta struct {
	ID          uuid.UUID `json:"id"`
	FolderID    uuid.UUID `json:"folder_id"`
	Name        string    `json:"name"`
	SizeBytes   int64     `json:"size_bytes"`
	ContentType string    `json:"content_type"`
	Checksum    string    `json:"checksum,omitempty"`
	OwnerSub    string    `json:"owner_sub,omitempty"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Preserved   bool      `json:"preserved"`
	// Label es la etiqueta de sensibilidad que llevaba el fichero (nil = sin
	// etiquetar). Es el snapshot guardado, no la taxonomía de hoy: eso es justo lo
	// que una investigación necesita saber.
	Label *domain.Label `json:"label,omitempty"`
}

func (a *complianceHandlers) ediscoverySearch(w http.ResponseWriter, r *http.Request) {
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
	subject := strings.TrimSpace(body.Subject)
	if subject == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Give the subject of the target user."))
		return
	}
	from, to, err := parseRange(body.From, body.To)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	hits, err := a.store.EDiscoverySearch(r.Context(), subject, store.EDiscoveryFilter{
		From: from, To: to, Name: body.Name, ContentType: body.ContentType,
		LabelID: strings.TrimSpace(body.LabelID),
		Limit:   body.Limit, Offset: body.Offset,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// Fail-closed: sin traza no se sirve el acceso privilegiado.
	if err := a.audit(r, claims.Subject, "ediscovery.search", subject, body.Case, map[string]any{
		"name": body.Name, "content_type": body.ContentType, "label_id": body.LabelID,
		"from": body.From, "to": body.To, "results": len(hits),
	}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]ediscoveryMeta, 0, len(hits))
	for _, h := range hits {
		f := h.File
		out = append(out, ediscoveryMeta{
			ID: f.ID, FolderID: f.FolderID, Name: f.Name, SizeBytes: f.SizeBytes,
			ContentType: f.ContentType, Checksum: f.Checksum, OwnerSub: f.OwnerSub,
			Version: f.Version, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
			Preserved: h.Preserved, Label: f.Label,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"target": subject,
		"total":  len(out),
		"files":  out,
	})
}

// ediscoveryExport streamea el conjunto de ficheros del sujeto como un ZIP (una
// sola pasada, sin buffer): el formato de export natural de ficheros.
// GET /api/v1/admin/ediscovery/export?subject=..&from=..&to=..&name=..&case=..
func (a *complianceHandlers) ediscoveryExport(w http.ResponseWriter, r *http.Request) {
	claims, err := requireAdmin(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := r.URL.Query()
	subject := strings.TrimSpace(q.Get("subject"))
	if subject == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Give the subject of the target user."))
		return
	}
	from, to, err := parseRange(q.Get("from"), q.Get("to"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if a.blobs == nil {
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("Object storage is unavailable, so the export cannot be produced."))
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	hits, err := a.store.EDiscoverySearch(r.Context(), subject, store.EDiscoveryFilter{
		From: from, To: to, Name: q.Get("name"), ContentType: q.Get("content_type"),
		LabelID: strings.TrimSpace(q.Get("label_id")), Limit: 5000,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// Fail-closed ANTES de escribir la cabecera 200 (una vez empezado el stream ya no
	// se puede cambiar el status).
	if err := a.audit(r, claims.Subject, "ediscovery.export", subject, q.Get("case"), map[string]any{
		"name": q.Get("name"), "content_type": q.Get("content_type"), "label_id": q.Get("label_id"),
		"from": q.Get("from"), "to": q.Get("to"), "results": len(hits), "format": "zip",
	}); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q",
		"ediscovery-"+sanitizeFilename(subject)+"-"+time.Now().UTC().Format("20060102")+".zip"))
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	defer zw.Close()
	seen := make(map[string]int, len(hits))
	for _, h := range hits {
		f := h.File
		blob, err := a.blobs.Get(r.Context(), t, f.StorageKey)
		if err != nil {
			// Blob ausente (p. ej. reapado por un GC externo): se anota y se continúa;
			// no aborta el export (el status 200 ya se envió).
			log.From(r.Context()).Warn("ediscovery export: blob no disponible", "file_id", f.ID, "error", err)
			continue
		}
		hdr := &zip.FileHeader{Name: zipEntryName(seen, f, h.Preserved), Method: zip.Deflate, Modified: f.UpdatedAt}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			_ = blob.Close()
			log.From(r.Context()).Warn("ediscovery export: crear entrada zip", "file_id", f.ID, "error", err)
			return
		}
		_, err = io.Copy(fw, blob)
		_ = blob.Close()
		if err != nil {
			log.From(r.Context()).Warn("ediscovery export: copiar blob", "file_id", f.ID, "error", err)
			return
		}
	}
}

// --- helpers --------------------------------------------------------------

// audit escribe la traza del acceso privilegiado. Devuelve ErrInternal si falla
// (el llamador es fail-closed: no sirve el resultado sin traza).
func (a *complianceHandlers) audit(r *http.Request, actor, action, target, caseRef string, detail map[string]any) error {
	detail["case"] = caseRef
	if err := a.store.WriteAudit(r.Context(), actor, action, target, detail); err != nil {
		log.From(r.Context()).Error("compliance: fallo al auditar acceso privilegiado", "action", action, "error", err)
		return httpx.ErrInternal.WithMessage("The audit trail could not be written, so the action was aborted.").Wrap(err)
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
	if b.Len() == 0 {
		return "export"
	}
	return b.String()
}

// zipEntryName da un nombre único a la entrada del ZIP (dos ficheros pueden llamarse
// igual en carpetas distintas). Prefija los preservados con "deleted/" para que el
// investigador distinga lo que el usuario había borrado. Desambigua colisiones con
// un sufijo por-nombre.
func zipEntryName(seen map[string]int, f domain.File, preserved bool) string {
	name := sanitizeFilename(f.Name)
	if preserved {
		name = "deleted/" + name
	}
	n := seen[name]
	seen[name] = n + 1
	if n == 0 {
		return name
	}
	return fmt.Sprintf("%s.%d", name, n)
}
