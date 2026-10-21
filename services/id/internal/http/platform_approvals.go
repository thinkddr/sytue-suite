// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Dos personas para lo irreversible (mig 0046).
//
// Suspender una organización corta el acceso de toda una empresa y fusionar no
// se puede deshacer. Hasta ahora bastaba un admin global elevado, y lo único que
// había en medio eran confirmaciones del tipo «repite el identificador»: eso
// protege del despiste, no de una cuenta comprometida ni de una decisión tomada
// a solas de madrugada.
//
// El flujo tiene DOS pasos y no tres: pedir crea la solicitud, y APROBAR
// EJECUTA. Dejar una solicitud «aprobada» a la espera de que alguien la lance
// añadiría un tercer momento en el que el mundo ya ha cambiado —la organización
// pudo fusionarse mientras tanto— sin que nadie vuelva a mirar.
package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Ventana de una solicitud. Un día: lo bastante para que el segundo admin esté
// en otra franja horaria, lo bastante poco para que nadie apruebe el martes algo
// que se pidió por una incidencia del viernes.
const platformApprovalTTL = 24 * time.Hour

// errPlatformSegundaPersona dice que la operación quedó ESPERANDO, no que haya
// fallado: son cosas distintas y el panel las enseña distinto.
var errPlatformSegundaPersona = httpx.ErrConflict.
	WithCode("approval_required").
	WithMessage("This operation needs approval from another global administrator.")

// payloadSuspend / payloadMerge son los parámetros que se guardan y se releen al
// aprobar. Se revalidan contra el estado de ESE momento, no el de cuando se pidió.
type payloadSuspend struct {
	OrgID  string `json:"org_id"`
	Reason string `json:"reason"`
}

type payloadMerge struct {
	SourceOrgID string `json:"source_org_id"`
	TargetOrgID string `json:"target_org_id"`
}

// payloadGrantAdmin son los parámetros de una concesión de alcance global. Se
// guarda el id y NO el correo: entre pedir y firmar cabe un día, y quien firma
// tiene que estar dando el alcance a la cuenta que se pidió, no a la que hoy
// tenga ese correo. El correo se relee al ejecutar, para el registro y el aviso.
type payloadGrantAdmin struct {
	UserID string `json:"user_id"`
	Reason string `json:"reason"`
}

// approvalJSON es la forma de una solicitud en el cable.
func approvalJSON(a store.PlatformApproval, now time.Time) map[string]any {
	out := map[string]any{
		"id":               a.ID,
		"action":           a.Action,
		"reason":           a.Reason,
		"requested_by":     a.RequestedBy,
		"requester_email":  a.RequesterEmail,
		"status":           a.Status,
		"effective_status": a.EffectiveStatus(now),
		"created_at":       a.CreatedAt.UTC().Format(time.RFC3339),
		"expires_at":       a.ExpiresAt.UTC().Format(time.RFC3339),
	}
	if len(a.Payload) > 0 {
		var p map[string]any
		if err := json.Unmarshal(a.Payload, &p); err == nil {
			out["payload"] = p
		}
	}
	if a.DecidedBy != "" {
		out["decided_by"] = a.DecidedBy
		out["decider_email"] = a.DeciderEmail
	}
	if a.DecidedAt != nil {
		out["decided_at"] = a.DecidedAt.UTC().Format(time.RFC3339)
	}
	if a.Result != "" {
		out["result"] = a.Result
	}
	return out
}

// pedirSegundaPersona registra la solicitud y responde 409 `approval_required`.
// Devuelve true si ya ha respondido (siempre que no haya que seguir adelante).
//
// El caso de UN SOLO admin global no es una excepción cómoda: con una sola
// persona elegible, exigir dos deja la plataforma sin poder suspender ni
// fusionar NUNCA, y eso se descubre el día que hace falta con urgencia. Se sigue
// adelante, y queda escrito en la auditoría que se hizo sin segunda firma.
func (s *Server) pedirSegundaPersona(w http.ResponseWriter, r *http.Request, action string, payload any, reason string) (seguir bool) {
	n, err := s.store.CountPlatformAdmins(r.Context())
	if err != nil {
		// Fail-closed: si no se puede saber cuántos hay, no se asume que hay uno.
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return false
	}
	if n <= 1 {
		s.auditPlatform(r, "platform.approval.solo", "", action+": "+reason)
		return true
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return false
	}
	a, err := s.store.CreatePlatformApproval(r.Context(), nil, action, raw, reason,
		platformActor(r), time.Now(), platformApprovalTTL)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return false
	}
	s.auditPlatform(r, "platform.approval.requested", "", action+": "+reason)
	// 409 con el cuerpo de la solicitud: el cliente necesita su id para poder
	// enseñarla, y el código distingue «esperando» de «ha fallado».
	httpx.JSON(w, r, http.StatusConflict, map[string]any{
		"error":    errPlatformSegundaPersona.Code,
		"code":     errPlatformSegundaPersona.Code,
		"message":  errPlatformSegundaPersona.Message,
		"approval": approvalJSON(a, time.Now()),
	})
	return false
}

// handlePlatformApprovals — GET /platform/approvals?pending=1 — solicitudes.
func (s *Server) handlePlatformApprovals(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	soloVivas := r.URL.Query().Get("pending") == "1"
	lista, err := s.store.ListPlatformApprovals(r.Context(), soloVivas, now, limit)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	items := make([]map[string]any, 0, len(lista))
	for _, a := range lista {
		items = append(items, approvalJSON(a, now))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// handlePlatformApprove — POST /platform/approvals/{id}/approve — la segunda
// firma, que además EJECUTA.
func (s *Server) handlePlatformApprove(w http.ResponseWriter, r *http.Request) {
	a, actor, ok := s.solicitudDecidible(w, r)
	if !ok {
		return
	}
	// El invariante entero de esto: quien aprueba no es quien pidió.
	if a.RequestedBy == actor {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("same_person").WithMessage(
			"You cannot approve your own request: another global administrator is needed.",
		))
		return
	}
	// Cerrar ANTES de ejecutar, y de forma atómica: si dos personas aprueban a la
	// vez, la segunda encuentra cero filas y no vuelve a fusionar.
	tomada, err := s.store.DecidePlatformApproval(r.Context(), nil, a.ID, "executed", actor, "", time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !tomada {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
			"This request has already been decided or has expired.",
		))
		return
	}
	s.auditPlatform(r, "platform.approval.approved", "", a.Action+": "+a.Reason)
	s.ejecutarAprobada(w, r, a)
}

// handlePlatformReject — POST /platform/approvals/{id}/reject.
//
// Rechazar SÍ puede hacerlo quien pidió: retirar la propia solicitud no ejecuta
// nada, y obligar a buscar a otro para deshacer un error sería absurdo.
func (s *Server) handlePlatformReject(w http.ResponseWriter, r *http.Request) {
	a, actor, ok := s.solicitudDecidible(w, r)
	if !ok {
		return
	}
	tomada, err := s.store.DecidePlatformApproval(r.Context(), nil, a.ID, "rejected", actor, "", time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !tomada {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
			"This request has already been decided or has expired.",
		))
		return
	}
	s.auditPlatform(r, "platform.approval.rejected", "", a.Action+": "+a.Reason)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"status": "rejected"})
}

// solicitudDecidible carga la solicitud de la URL y el actor. Si no procede, ya
// ha respondido.
func (s *Server) solicitudDecidible(w http.ResponseWriter, r *http.Request) (store.PlatformApproval, string, bool) {
	a, err := s.store.GetPlatformApproval(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return a, "", false
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return a, "", false
	}
	if a.EffectiveStatus(time.Now()) != "pending" {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
			"This request has already been decided or has expired.",
		))
		return a, "", false
	}
	return a, platformActor(r), true
}

// ejecutarAprobada lleva a cabo la operación de una solicitud recién aprobada.
//
// Se revalida TODO contra el estado de ahora: entre la petición y la firma pudo
// pasar un día entero, y la organización pudo fusionarse, suspenderse o dejar de
// existir. Si la operación ya no procede, la solicitud queda `executed` con el
// motivo escrito en `result`: reabrirla como pendiente dejaría una firma dada
// para algo que no se hizo.
func (s *Server) ejecutarAprobada(w http.ResponseWriter, r *http.Request, a store.PlatformApproval) {
	switch a.Action {
	case "org.suspend":
		var p payloadSuspend
		if err := json.Unmarshal(a.Payload, &p); err != nil {
			s.fallaAprobada(r, a, "solicitud ilegible")
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		org, err := s.aplicarEstadoOrg(r, p.OrgID, domain.StatusSuspended, p.Reason)
		if err != nil {
			s.fallaAprobada(r, a, err.Error())
			httpx.Fail(w, r, mapDomainErr(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{
			"id": org.ID, "slug": org.Slug, "status": string(org.Status),
		})
	case "org.delete":
		// Mismo cuerpo que `org.suspend` y a propósito: eliminar es poner el estado
		// `deleted`, no borrar filas (el porqué está en handlePlatformSetOrgStatus).
		// Se revalida contra el estado de AHORA como el resto: entre la petición y
		// la firma la organización pudo fusionarse o dejar de existir.
		var p payloadSuspend
		if err := json.Unmarshal(a.Payload, &p); err != nil {
			s.fallaAprobada(r, a, "solicitud ilegible")
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		org, err := s.aplicarEstadoOrg(r, p.OrgID, domain.StatusDeleted, p.Reason)
		if err != nil {
			s.fallaAprobada(r, a, err.Error())
			httpx.Fail(w, r, mapDomainErr(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{
			"id": org.ID, "slug": org.Slug, "status": string(org.Status),
		})
	case "org.merge":
		var p payloadMerge
		if err := json.Unmarshal(a.Payload, &p); err != nil {
			s.fallaAprobada(r, a, "solicitud ilegible")
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		m, err := s.arrancarFusion(r, p.SourceOrgID, p.TargetOrgID)
		if err != nil {
			s.fallaAprobada(r, a, err.Error())
			httpx.Fail(w, r, mapDomainErr(err))
			return
		}
		if err := s.store.SetPlatformApprovalResult(r.Context(), nil, a.ID, "merge:"+m.ID); err != nil {
			s.log.Error("no se pudo anotar el resultado de la aprobación", "id", a.ID, "error", err)
		}
		httpx.JSON(w, r, http.StatusAccepted, mergeWire(m))
	case "platform.admin.grant":
		var p payloadGrantAdmin
		if err := json.Unmarshal(a.Payload, &p); err != nil {
			s.fallaAprobada(r, a, "solicitud ilegible")
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		// El usuario se relee AHORA: pudo darse de baja en el día que cabe entre
		// pedir y firmar, y conceder el alcance global a una cuenta que ya no
		// existe dejaría una fila apuntando a nadie.
		u, err := s.store.GetUserByID(r.Context(), p.UserID)
		if err != nil {
			s.fallaAprobada(r, a, "la cuenta ya no existe")
			httpx.Fail(w, r, mapDomainErr(err))
			return
		}
		if err := s.concederAlcance(r, u.ID, u.Email, p.Reason); err != nil {
			s.fallaAprobada(r, a, err.Error())
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		httpx.JSON(w, r, http.StatusCreated, map[string]any{"user_id": u.ID, "email": u.Email})
	default:
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("Unknown action."))
	}
}

// fallaAprobada deja escrito por qué una solicitud aprobada no llegó a hacerse.
func (s *Server) fallaAprobada(r *http.Request, a store.PlatformApproval, motivo string) {
	if err := s.store.SetPlatformApprovalResult(r.Context(), nil, a.ID, "error: "+motivo); err != nil {
		s.log.Error("no se pudo anotar el fallo de la aprobación", "id", a.ID, "error", err)
	}
}
