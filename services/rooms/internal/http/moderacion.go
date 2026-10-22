// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Denunciar y bloquear (ADR 0073).
//
// EL ACTOR SALE SIEMPRE DEL TOKEN: ninguna de estas rutas acepta en el cuerpo ni
// en la ruta «quién denuncia» o «quién bloquea», así que no hay forma de hacerlo
// en nombre de otro. Lo que sí viene de fuera —el mensaje, el canal, la persona—
// lo comprueba el almacén contra lo que ese actor puede ver.
package chathttp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/operador"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// moderacionService es lo que estas rutas piden al servicio. Interfaz propia,
// como `adminStore`: `chatService` la implementan los dobles de todos los tests de
// handlers y ampliarla obligaría a tocarlos para métodos que no usan.
type moderacionService interface {
	Report(ctx context.Context, rec store.NewReport) (uuid.UUID, error)
	ListReports(ctx context.Context, actor string, orgAdmin, abiertas bool) ([]domain.Report, error)
	ResolveReport(ctx context.Context, id uuid.UUID, actor string, orgAdmin bool, action string) (domain.Report, error)
	BlockMember(ctx context.Context, channelID uuid.UUID, actor, subject string) (domain.Block, error)
	ListBlocks(ctx context.Context, actor string) ([]domain.Block, error)
	Unblock(ctx context.Context, actor, subject string) error
}

// operadorService es lo que pide la cola del operador de la plataforma. Aparte,
// como moderacionService respecto a chatService.
type operadorService interface {
	ListReportsOperador(ctx context.Context, actor string, abiertas bool) ([]domain.Report, error)
	OrgDeDenuncia(ctx context.Context, id uuid.UUID) (string, error)
	ResolveReportOperador(ctx context.Context, org string, id uuid.UUID, actor, action string) (domain.Report, error)
}

// ModeracionHandlers sirve las rutas de denuncia, bloqueo y cola de moderación.
type ModeracionHandlers struct {
	svc moderacionService
	// op y autorizar son la cola del operador; con cualquiera de los dos a nil,
	// esa cola contesta 403.
	op        operadorService
	autorizar operador.Autorizar
}

// NewModeracionHandlers construye los handlers.
func NewModeracionHandlers(svc moderacionService) *ModeracionHandlers {
	return &ModeracionHandlers{svc: svc}
}

// ConOperador abre la cola del operador: `autorizar` pregunta a Sytue ID.
func (h *ModeracionHandlers) ConOperador(op operadorService, autorizar operador.Autorizar) *ModeracionHandlers {
	h.op, h.autorizar = op, autorizar
	return h
}

type reportRequest struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// ReportMessage — POST /messages/{messageID}/report — denuncia un mensaje y, con
// él, a su autor.
func (h *ModeracionHandlers) ReportMessage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.report(w, r, store.NewReport{MessageID: &id})
}

// ReportMember — POST /channels/{channelID}/members/{sub}/report — denuncia a una
// persona en el canal que se comparte con ella.
func (h *ModeracionHandlers) ReportMember(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.report(w, r, store.NewReport{ChannelID: id, TargetSub: chi.URLParam(r, "sub")})
}

func (h *ModeracionHandlers) report(w http.ResponseWriter, r *http.Request, rec store.NewReport) {
	var req reportRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rec.Reporter, _ = caller(r)
	rec.Reason, rec.Note = req.Reason, req.Note
	id, err := h.svc.Report(r.Context(), rec)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// 200 también cuando ya estaba denunciado: para quien denuncia es lo mismo.
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id})
}

// Block — PUT /channels/{channelID}/members/{sub}/block — bloquea a alguien con
// quien se comparte el canal.
func (h *ModeracionHandlers) Block(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	b, err := h.svc.BlockMember(r.Context(), id, actor, chi.URLParam(r, "sub"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, b)
}

// ListBlocks — GET /me/blocks — a quién tiene bloqueado quien llama.
func (h *ModeracionHandlers) ListBlocks(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	items, err := h.svc.ListBlocks(r.Context(), actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Unblock — DELETE /me/blocks/{sub} — desbloquea.
func (h *ModeracionHandlers) Unblock(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(chi.URLParam(r, "sub"))
	if target == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid subject."))
		return
	}
	actor, _ := caller(r)
	if err := h.svc.Unblock(r.Context(), actor, target); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// moderador devuelve quién llama y si administra la ORGANIZACIÓN.
//
// El rol sale de los claims, que son los de la organización del TOKEN, y la cola
// es la de esa organización: estas rutas no cambian de organización. En una ruta
// que sí cambia (`enAnfitrionPor`) los roles llegan ya vacíos —EnAnfitrion los
// quita—, así que tampoco ahí el administrador de una organización modera otra.
func moderador(r *http.Request) (string, bool) {
	c, _ := authn.FromContext(r.Context())
	return c.Subject, c.HasAnyRole("owner", "admin")
}

// ListReports — GET /moderation/reports — la cola de denuncias de quien modera:
// las de toda la organización para sus administradores, y las de sus equipos para
// quien administra un equipo. `?all=1` incluye las resueltas.
func (h *ModeracionHandlers) ListReports(w http.ResponseWriter, r *http.Request) {
	actor, orgAdmin := moderador(r)
	items, err := h.svc.ListReports(r.Context(), actor, orgAdmin, r.URL.Query().Get("all") != "1")
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type resolveRequest struct {
	// Action es el estado en que queda: dismissed | message_deleted | member_removed.
	Action string `json:"action"`
}

// ResolveReport — POST /moderation/reports/{reportID}/resolve.
func (h *ModeracionHandlers) ResolveReport(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "reportID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req resolveRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, orgAdmin := moderador(r)
	rep, err := h.svc.ResolveReport(r.Context(), id, actor, orgAdmin, req.Action)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, rep)
}

// ── La cola del operador ────────────────────────────────────────────────────

// operadorDe: solo con un token de la consola de plataforma —mirado antes de
// tocar la base— y con el sí de Sytue ID, que recibe qué se va a hacer y deja la
// traza. `org` se rellena antes de preguntar si hace falta (resolver). Si no es
// que sí, ya ha respondido.
func (h *ModeracionHandlers) deLaConsola(w http.ResponseWriter, r *http.Request) (string, bool) {
	c, _ := authn.FromContext(r.Context())
	if c.Subject == "" || !operador.DeLaConsola(c.Audience) || h.op == nil || h.autorizar == nil {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return "", false
	}
	return c.Subject, true
}

func (h *ModeracionHandlers) operadorDe(w http.ResponseWriter, r *http.Request, a operador.Acceso) bool {
	a.Servicio = "rooms"
	if err := h.autorizar(r.Context(), authn.Token(r), a); err != nil {
		if errors.Is(err, operador.ErrNoEsOperador) {
			httpx.Fail(w, r, httpx.ErrForbidden)
		} else {
			httpx.Fail(w, r, httpx.ErrUnavailable.Wrap(err))
		}
		return false
	}
	return true
}

// ListReportsOperador — GET /platform/moderation/reports — la cola de todas las
// organizaciones. `?all=1` incluye las resueltas.
func (h *ModeracionHandlers) ListReportsOperador(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.deLaConsola(w, r)
	if !ok {
		return
	}
	abiertas := r.URL.Query().Get("all") != "1"
	estado := "open"
	if !abiertas {
		estado = "all"
	}
	if !h.operadorDe(w, r, operador.Acceso{Accion: "reports.list", Detalle: map[string]string{"status": estado}}) {
		return
	}
	items, err := h.op.ListReportsOperador(r.Context(), actor, abiertas)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// ResolveReportOperador — POST /platform/moderation/reports/{reportID}/resolve.
func (h *ModeracionHandlers) ResolveReportOperador(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "reportID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req resolveRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, ok := h.deLaConsola(w, r)
	if !ok {
		return
	}
	// La organización, antes de preguntar, para que la traza diga sobre cuál; y se
	// pregunta también si no existe, para no contestar «no existe» a quien no es
	// operador.
	org, err := h.op.OrgDeDenuncia(r.Context(), id)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if !h.operadorDe(w, r, operador.Acceso{
		Accion: "report.resolve", Org: org, Objetivo: id.String(), Detalle: map[string]string{"action": req.Action},
	}) {
		return
	}
	if org == "" {
		httpx.Fail(w, r, toAPIError(domain.ErrNotFound))
		return
	}
	rep, err := h.op.ResolveReportOperador(r.Context(), org, id, actor, req.Action)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, rep)
}
