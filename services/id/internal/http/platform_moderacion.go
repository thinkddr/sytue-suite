// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// COLA DE DENUNCIAS DEL OPERADOR (ADR 0073 y 0074, «Cola del operador»).
//
// Las colas viven en Cast y en Rooms, pero quién es operador lo sabe Sytue ID:
// allowlist, segundo factor y elevación viva, comprobados en cada petición por
// `requirePlatformAdmin`. Antes de enseñar o tocar una denuncia, el servicio
// llama aquí con el token de quien pide y lo que va a hacer; esta ruta lo deja en
// el registro de plataforma y contesta 204. Si no contesta 204, el servicio no
// enseña nada (platform/pkg/operador).
//
// Así la comprobación y la traza son la misma llamada, y no hay forma de leer una
// denuncia ajena sin dejar rastro: si no se puede escribir el registro, no hay
// lectura. Con la organización afectada, la fila sale también en el registro de
// ESA organización: su administración ve que Sytue resolvió una denuncia suya.

// Lo que un servicio puede declarar. Cerrado: el registro lo leen personas y una
// acción inventada no la entendería nadie.
var (
	moderacionServicios = map[string]bool{"cast": true, "rooms": true}
	moderacionAcciones  = map[string]bool{"reports.list": true, "report.resolve": true}
)

type moderacionAccesoRequest struct {
	Service string `json:"service"`
	Action  string `json:"action"`
	// Org es el slug de la organización dueña de lo que se toca; vacío al listar
	// la cola, que es de todas.
	Org    string            `json:"org"`
	Target string            `json:"target"`
	Detail map[string]string `json:"detail"`
}

// handlePlatformModeracionAcceso — POST /api/v1/platform/moderation/access.
func (s *Server) handlePlatformModeracionAcceso(w http.ResponseWriter, r *http.Request) {
	var req moderacionAccesoRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !moderacionServicios[req.Service] || !moderacionAcciones[req.Action] ||
		(req.Org != "" && !tenancy.ValidSlug(req.Org)) ||
		utf8.RuneCountInString(req.Target) > 100 || len(req.Detail) > 8 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown service, action or organization."))
		return
	}
	c, _ := claimsFrom(r.Context())
	detalle := map[string]any{"from_org": c.Tenant, "client": c.ClientID}
	for k, v := range req.Detail {
		if utf8.RuneCountInString(k) > 40 || utf8.RuneCountInString(v) > 200 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Detail too long."))
			return
		}
		if _, ya := detalle[k]; !ya {
			detalle[k] = v
		}
	}
	orgID := ""
	if req.Org != "" {
		org, err := s.store.GetOrgBySlug(r.Context(), req.Org)
		switch {
		case err == nil:
			orgID = org.ID
		case errors.Is(err, domain.ErrNotFound):
			// Una organización ya borrada sigue pudiendo tener denuncias en Cast o
			// Rooms: la fila va sin org_id y con el slug en el detalle.
			detalle["org"] = req.Org
		default:
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
	}
	e := store.AuditEntry{
		ActorID: c.Subject, OrgID: orgID, Action: "platform.moderation." + req.Service + "." + req.Action,
		Target: req.Target, IP: clientIP(r), Detail: detalle,
	}
	// Sin registro no hay acceso: a diferencia de auditPlatform, un fallo aquí
	// DENIEGA, porque lo que se pide es leer contenido de un cliente.
	if err := s.store.Audit(r.Context(), nil, e); err != nil {
		s.log.Error("platform: no se pudo registrar un acceso de moderación", "action", e.Action, "error", err)
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	httpx.NoContent(w)
}
