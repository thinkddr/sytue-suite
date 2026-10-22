// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Acciones admin sobre VARIOS usuarios a la vez.
//
// Existe porque las de uno en uno no escalan a la realidad: una baja de
// departamento son treinta personas, y un incidente de seguridad son todas las
// que tocaron algo. Hacerlo a mano treinta veces no es solo lento — a la
// vigesimoquinta se salta una, y esa es justo la que importaba.
//
// Dos decisiones que gobiernan este fichero:
//
//   - CADA USUARIO SE RESUELVE POR SU CUENTA. No hay transacción común ni
//     «todo o nada»: en un lote de treinta, dos van a fallar por motivos
//     legítimos —uno es owner y quien pide es admin, otro eres tú— y abortar los
//     veintiocho restantes por eso convierte la función en inútil. La respuesta
//     dice qué pasó con cada uno.
//   - LOS GUARDAS SON LOS MISMOS, uno a uno. `adminMayManage` y los anti-lockout
//     de rol se aplican por usuario, con los recuentos en vivo dentro de su
//     propia transacción: un lote no es una vía para saltarse lo que la acción
//     individual no deja hacer.
package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// bulkMax acota el lote. 200 es lo que cabe en una pantalla de selección
// razonable y deja la petición dentro del timeout de 30 s del router aunque cada
// usuario lleve su propia transacción.
const bulkMax = 200

type bulkRequest struct {
	Action  string   `json:"action"`
	UserIDs []string `json:"user_ids"`
	// Solo para action=set_role.
	Role string `json:"role"`
}

// bulkResult es lo que pasó con UN usuario. `code` es estable y traducible en el
// cliente; `message` no viaja: el catálogo de los cinco idiomas está en el
// frontend (CONVENTIONS §13.1).
type bulkResult struct {
	ID   string `json:"id"`
	OK   bool   `json:"ok"`
	Code string `json:"code,omitempty"`
	// Sesiones cerradas por esta acción sobre este usuario.
	Revoked int `json:"sessions_revoked,omitempty"`
}

// handleAdminBulkUsers — POST /api/v1/admin/users/bulk.
//
// Responde SIEMPRE 200 con el detalle por usuario, incluso si fallan todos: el
// resultado de un lote no es un éxito ni un fracaso, es una lista. Un 4xx global
// obligaría al cliente a adivinar cuáles se aplicaron.
func (s *Server) handleAdminBulkUsers(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req bulkRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	switch req.Action {
	case "suspend", "reactivate", "revoke_sessions", "set_role":
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"Invalid action (suspend|reactivate|revoke_sessions|set_role).",
		))
		return
	}
	if len(req.UserIDs) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("No has seleccionado a nadie."))
		return
	}
	if len(req.UserIDs) > bulkMax {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"Too many users at once (200 maximum).",
		))
		return
	}
	nuevoRol := domain.Role(req.Role)
	if req.Action == "set_role" {
		if !nuevoRol.Valid() {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid role (owner|admin|member|billing)."))
			return
		}
		// Mismo techo que en la acción individual: solo un owner concede owner.
		if nuevoRol == domain.RoleOwner && !hasAnyRole(c.Roles, domain.RoleOwner) {
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Only an owner can assign the owner role."))
			return
		}
	}

	// Ids repetidos: se procesa cada uno UNA vez. Repetir «suspender» sobre el
	// mismo usuario no rompe nada, pero el recuento de la respuesta mentiría.
	vistos := make(map[string]bool, len(req.UserIDs))
	unicos := make([]string, 0, len(req.UserIDs))
	for _, id := range req.UserIDs {
		if !vistos[id] {
			vistos[id] = true
			unicos = append(unicos, id)
		}
	}

	// EL LOTE EXISTE COMO UNA COSA, desde antes de tocar a nadie. Sin la fila,
	// quien lo lanzó se quedaba delante de una petición abierta sin saber por
	// dónde iba, y equivocarse de selección no tenía más salida que cerrar la
	// pestaña — que no para nada, porque el servidor sigue.
	ctx := r.Context()
	trabajo, err := s.store.CrearTrabajo(ctx, org.ID, req.Action, c.Subject, len(unicos))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}

	out := make([]bulkResult, 0, len(unicos))
	cancelado := false
	for _, id := range unicos {
		out = append(out, s.bulkUno(r, org, c.Subject, c.Roles, id, req.Action, nuevoRol))
		// El progreso se escribe y la cancelación se lee en la MISMA sentencia:
		// separarlas sería una carrera contra quien cancela justo en medio.
		parar, errAv := s.store.AvanzarTrabajo(ctx, trabajo, out)
		if errAv != nil {
			// Un fallo al apuntar el progreso no tira el lote: lo que importa es
			// lo que se le hace a la gente, no el contador.
			s.log.WarnContext(ctx, "bulk: no se pudo apuntar el progreso", "job", trabajo, "error", errAv)
			continue
		}
		if parar {
			cancelado = true
			break
		}
	}

	estado := "terminado"
	if cancelado {
		estado = "cancelado"
	}
	if err := s.store.CerrarTrabajo(ctx, trabajo, estado, out); err != nil {
		s.log.WarnContext(ctx, "bulk: no se pudo cerrar el trabajo", "job", trabajo, "error", err)
	}

	// Una sola entrada de auditoría por lote, con el recuento: treinta filas
	// idénticas separadas por milisegundos no se leen mejor que una que dice
	// «suspendidos 28 de 30».
	hechos := 0
	for _, res := range out {
		if res.OK {
			hechos++
		}
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.users.bulk."+req.Action,
		strconv.Itoa(hechos)+"/"+strconv.Itoa(len(unicos)), r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"job_id": trabajo, "status": estado,
		"results": out, "applied": hechos,
		// Cuántos quedaron SIN TOCAR al cancelar. Es el número que hay que
		// enseñar: «paré a tiempo» y «llegué tarde» no son lo mismo.
		"not_processed": len(unicos) - len(out),
	})
}

// handleAdminListJobs — GET /api/v1/admin/jobs — los lotes recientes.
func (s *Server) handleAdminListJobs(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	lista, err := s.store.Trabajos(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	ahora := time.Now()
	out := make([]map[string]any, 0, len(lista))
	for _, t := range lista {
		out = append(out, trabajoJSON(t, ahora))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"jobs": out})
}

// handleAdminGetJob — GET /api/v1/admin/jobs/{id} — progreso e informe por
// elemento. Es lo que consulta quien está mirando cómo va.
func (s *Server) handleAdminGetJob(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid job id."))
		return
	}
	t, err := s.store.Trabajo(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	cuerpo := trabajoJSON(t, time.Now())
	cuerpo["results"] = t.Resultados
	httpx.JSON(w, r, http.StatusOK, cuerpo)
}

// handleAdminCancelJob — POST /api/v1/admin/jobs/{id}/cancel — pide parar.
//
// PIDE, no mata: el bucle la obedece antes del siguiente elemento. Cortar por
// la espalda dejaría una acción a medias sin cerrar su fila, que es peor que
// tocar una persona de más.
func (s *Server) handleAdminCancelJob(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid job id."))
		return
	}
	pedida, err := s.store.PedirCancelacionTrabajo(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !pedida {
		// Ni error ni éxito inventado: no había nada corriendo que parar.
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("That job is not running any more."))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.users.bulk.cancel", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "cancel_requested": true})
}

func trabajoJSON(t store.TrabajoMasivo, ahora time.Time) map[string]any {
	return map[string]any{
		"id": t.ID, "action": t.Accion,
		"total": t.Total, "done": t.Hechos,
		// El estado VISIBLE, no el de la fila: un lote cuya petición murió con
		// el pod se queda en «ejecutando» para siempre, y decir que sigue
		// corriendo es mentir.
		"status":           t.EstadoVisible(ahora),
		"cancel_requested": t.Cancelando,
		"created_at":       t.CreadoEn, "finished_at": t.TerminadoEn,
	}
}

// bulkUno aplica la acción a UN usuario y traduce el fallo a un código estable.
// Nunca devuelve error: en un lote, el fallo de uno es un dato, no una avería.
func (s *Server) bulkUno(r *http.Request, org domain.Organization, actorID string, actorRoles []string, id, action string, nuevoRol domain.Role) bulkResult {
	ctx := r.Context()
	if !uuidRe.MatchString(id) {
		return bulkResult{ID: id, Code: "not_found"}
	}
	// El objetivo tiene que ser miembro de la organización activa: un id de otra
	// organización no es un error del lote, es alguien que no está aquí.
	mem, errMiembro := s.store.GetMembership(ctx, org.ID, id)
	if errMiembro != nil {
		return bulkResult{ID: id, Code: "not_found"}
	}
	if e := adminMayManage(actorID, id, actorRoles, mem.Role); e != nil {
		// Los dos motivos se distinguen porque llevan a cosas distintas: quitarse
		// a uno mismo del lote, o pedírselo a un owner.
		code := "forbidden"
		if actorID == id {
			code = "self"
		}
		return bulkResult{ID: id, Code: code}
	}

	switch action {
	case "suspend":
		return s.bulkSuspender(ctx, id)

	case "reactivate":
		ok, e := s.store.SetUserStatus(ctx, nil, id, domain.StatusActive)
		if e != nil {
			return bulkResult{ID: id, Code: "error"}
		}
		if !ok {
			e := s.store.ReactivateClosedUser(ctx, nil, id, time.Now().Add(-retencionBaja))
			if errors.Is(e, store.ErrNoRecuperable) {
				return bulkResult{ID: id, Code: "deleted"}
			}
			if e != nil {
				return bulkResult{ID: id, Code: "error"}
			}
			_ = s.store.Audit(ctx, nil, auditEntry(actorID, org.ID, "org.user.restored", id, r))
		}
		return bulkResult{ID: id, OK: true}

	case "revoke_sessions":
		n, e := s.store.RevokeAllUserSessions(ctx, nil, id)
		if e != nil {
			return bulkResult{ID: id, Code: "error"}
		}
		return bulkResult{ID: id, OK: true, Revoked: n}

	case "set_role":
		if nuevoRol == mem.Role {
			// Ya lo tiene: no es un fallo ni una acción. Cuenta como hecho, que es
			// el estado que se pedía.
			return bulkResult{ID: id, OK: true}
		}
		revoked, e := s.aplicarRol(ctx, org.ID, id, mem.Role, nuevoRol)
		switch {
		case errors.Is(e, errLastOwner):
			return bulkResult{ID: id, Code: "last_owner"}
		case errors.Is(e, errLastAdmin):
			return bulkResult{ID: id, Code: "last_admin"}
		case e != nil:
			return bulkResult{ID: id, Code: "error"}
		}
		return bulkResult{ID: id, OK: true, Revoked: revoked}
	}
	return bulkResult{ID: id, Code: "error"}
}

// bulkSuspender da de baja a un usuario del lote.
//
// Las sesiones vivas se CUENTAN antes de cambiar el estado y dentro de la misma
// transacción: SetUserStatus las cierra por dentro, así que preguntarlo después
// devolvería cero y la respuesta no podría decir cuántas se cortaron —que es el
// dato por el que se suspende a alguien en un incidente—.
func (s *Server) bulkSuspender(ctx context.Context, id string) bulkResult {
	var revoked int
	var updated bool
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		n, e := s.store.CountLiveSessions(ctx, q, id)
		if e != nil {
			return e
		}
		ok, e := s.store.SetUserStatus(ctx, q, id, domain.StatusSuspended)
		if e != nil {
			return e
		}
		updated = ok
		if !ok {
			return nil
		}
		revoked = n
		return nil
	})
	if err != nil {
		return bulkResult{ID: id, Code: "error"}
	}
	if !updated {
		return bulkResult{ID: id, Code: "deleted"}
	}
	return bulkResult{ID: id, OK: true, Revoked: revoked}
}
