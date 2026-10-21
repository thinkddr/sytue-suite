// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Acciones admin de Sytue Admin sobre un usuario de la organización activa:
// suspender/reactivar la cuenta y listar/cerrar sus sesiones (dispositivos). Son
// las contrapartidas de las acciones self-service de /account, pero ejecutadas
// por un admin sobre OTRO usuario.
//
// Aislamiento por tenant (docs/CONVENTIONS.md §0.7): requireRole ya autoriza al
// llamante en su org activa (c.Tenant); estas rutas SIEMPRE derivan la org de ese
// claim (activeOrg), nunca del path, y exigen que el usuario objetivo sea miembro
// de ESA org (GetMembership) antes de tocar nada — un id de otra org da 404, no
// una acción cross-tenant (anti-IDOR). Suspender bloquea el acceso de inmediato
// (login/refresh miran users.status) y, además, revoca sesiones y refresh ya
// emitidos para que un access token vivo no sobreviva a la suspensión.
package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// adminMayManage decide si el llamante puede aplicar una acción de "corte"
// (suspender / cerrar sesiones) sobre el usuario objetivo. Dos invariantes:
//   - No sobre uno mismo: evita que un admin se autobloquee o se cierre su propia
//     sesión desde la Console (foot-gun) — para eso está /account.
//   - Un owner solo lo toca otro owner: un admin no puede cortar el acceso de un
//     owner (evita que un admin degrade/expulse a quien está por encima).
//
// Devuelve *httpx.Error (nil = permitido); el tipo concreto permite el patrón
// `if e := ...; e != nil` sin la trampa del interface nil.
func adminMayManage(callerID, targetID string, callerRoles []string, targetRole domain.Role) *httpx.Error {
	if callerID == targetID {
		return httpx.ErrForbidden.WithMessage("You cannot do this to your own account; use My account.")
	}
	if targetRole == domain.RoleOwner && !hasAnyRole(callerRoles, domain.RoleOwner) {
		return httpx.ErrForbidden.WithMessage("Only an owner can do this to another owner.")
	}
	return nil
}

// adminTargetInOrg resuelve la org activa y verifica que {id} sea un uuid y un
// miembro de ella, devolviendo la membresía (rol) del objetivo. Concentra el
// gate anti-IDOR compartido por las cuatro acciones.
func (s *Server) adminTargetInOrg(r *http.Request) (org domain.Organization, targetID string, mem domain.Membership, apiErr error) {
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		return org, "", mem, httpx.ErrBadRequest.WithMessage("Invalid user id.")
	}
	org, err := s.activeOrg(r)
	if err != nil {
		return org, "", mem, err
	}
	mem, err = s.store.GetMembership(r.Context(), org.ID, id)
	if err != nil {
		// No miembro de la org activa ⇒ 404 (no revelar existencia cross-tenant).
		return org, "", mem, mapDomainErr(err)
	}
	return org, id, mem, nil
}

// handleAdminSuspendUser — POST /api/v1/admin/users/{id}/suspend — suspende la
// cuenta y revoca sus sesiones y refresh tokens en la misma transacción.
func (s *Server) handleAdminSuspendUser(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, id, mem, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if e := adminMayManage(c.Subject, id, c.Roles, mem.Role); e != nil {
		httpx.Fail(w, r, e)
		return
	}
	var revoked int
	var updated bool
	ctx := r.Context()
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		// El recuento va ANTES: SetUserStatus ya arrastra las sesiones por dentro
		// (revokeIfInactive), así que preguntarlo después devolvía siempre cero y
		// el «se cerraron N sesiones» de la consola no salía nunca.
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
			return nil // cuenta dada de baja: nada que revocar
		}
		revoked = n
		return nil
	})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !updated {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("This account has been closed."))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "user.suspended", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "status": "suspended", "sessions_revoked": revoked})
}

// handleAdminReactivateUser — POST /api/v1/admin/users/{id}/reactivate — vuelve a
// activar la cuenta. No reabre sesiones (el usuario inicia sesión de nuevo).
func (s *Server) handleAdminReactivateUser(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, id, _, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	ok, err := s.store.SetUserStatus(r.Context(), nil, id, domain.StatusActive)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !ok {
		// Dada de baja: reactivar la restaura y la activa a la vez, dentro de
		// la ventana y salvo baja propia (ver store.ReactivateClosedUser).
		e := s.store.ReactivateClosedUser(r.Context(), nil, id, time.Now().Add(-retencionBaja))
		if errors.Is(e, store.ErrNoRecuperable) {
			httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
				"This account cannot be reactivated: its recovery window has passed, or the person closed it themselves."))
			return
		}
		if e != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(e))
			return
		}
		_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.user.restored", id, r))
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "user.reactivated", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "status": "active"})
}

// handleAdminResetMFA — POST /api/v1/admin/users/{id}/reset-mfa — borra TODO el
// segundo factor del usuario (TOTP + passkeys + códigos de recuperación) para que
// re-enrole (recuperación de cuenta / dispositivo perdido). Idempotente (sin MFA no
// es error). Mismo anclaje que las demás acciones admin: adminTargetInOrg aísla por
// tenant (id de otra org ⇒ 404) y adminMayManage añade las invariantes (no sobre
// uno mismo → usa /account; a un owner solo otro owner). No hace falta guard de
// "último admin": resetear el MFA no borra la cuenta ni cambia el rol.
func (s *Server) handleAdminResetMFA(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, id, mem, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if e := adminMayManage(c.Subject, id, c.Roles, mem.Role); e != nil {
		httpx.Fail(w, r, e)
		return
	}
	ctx := r.Context()
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		return s.store.ResetUserMFA(ctx, q, org.ID, id)
	})
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.user.mfa_reset", id, r))
	s.avisarSeguridad(ctx, org.ID, id, "security.mfa.disabled", nil)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true})
}

// handleAdminUserSessions — GET /api/v1/admin/users/{id}/sessions — sesiones SSO
// vigentes del usuario (visibilidad de dispositivos para soporte/seguridad).
func (s *Server) handleAdminUserSessions(w http.ResponseWriter, r *http.Request) {
	_, id, _, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	// currentTokenHash="": el admin no es el usuario, ninguna sesión es "la actual".
	list, err := s.store.ListSessionsForUser(r.Context(), id, "")
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, ss := range list {
		out = append(out, map[string]any{
			"id":         ss.ID,
			"ip":         ss.IP,
			"user_agent": ss.UserAgent,
			"device":     deviceES(i18n.From(r), ss.UserAgent),
			"amr":        ss.AMR,
			"auth_time":  ss.AuthTime,
			"expires_at": ss.ExpiresAt,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"sessions": out})
}

// handleAdminUserImpact — GET /api/v1/admin/users/{id}/impact — qué pasa de
// verdad si se toca a este usuario.
//
// Existe porque las confirmaciones de la consola eran genéricas: «¿seguro?»
// delante de una acción no dice nada que quien la pulsa no supiera ya. Lo que
// hace dudar a tiempo es el número —«se cerrarán 4 sesiones», «es el único
// propietario»—, y ese número solo lo tiene el servidor.
//
// Es una LECTURA y va antes de actuar; el estado puede cambiar entre la consulta
// y la acción, y no pasa nada: informa una decisión, no la autoriza. Quien
// autoriza sigue siendo cada endpoint.
func (s *Server) handleAdminUserImpact(w http.ResponseWriter, r *http.Request) {
	org, id, mem, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	sesiones, err := s.store.ListSessionsForUser(r.Context(), id, "")
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	grupos, err := s.store.GroupsForUser(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// «Es el ÚLTIMO propietario» es la pregunta que evita dejar una organización
	// sin nadie que pueda administrarla: se responde aquí y no en el cliente,
	// que no puede contar los owners de una organización entera. La membresía ya
	// viene del gate anti-IDOR; volver a leerla sería una consulta de más.
	esUltimoOwner := false
	if mem.Role == domain.RoleOwner {
		n, errC := s.store.CountOrgOwners(r.Context(), nil, org.ID, id)
		if errC != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(errC))
			return
		}
		esUltimoOwner = n <= 1
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"active_sessions": len(sesiones),
		"groups":          len(grupos),
		"last_owner":      esUltimoOwner,
	})
}

// handleAdminRevokeUserSessions — POST /api/v1/admin/users/{id}/sessions/revoke —
// cierre forzado de TODAS las sesiones del usuario (cuenta comprometida), con
// cascada sobre sus refresh tokens. No cambia el estado de la cuenta.
func (s *Server) handleAdminRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, id, mem, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if e := adminMayManage(c.Subject, id, c.Roles, mem.Role); e != nil {
		httpx.Fail(w, r, e)
		return
	}
	var revoked int
	ctx := r.Context()
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		n, e := s.store.RevokeAllUserSessions(ctx, q, id)
		revoked = n
		return e
	})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if revoked > 0 {
		_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "user.sessions_revoked", id, r))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "sessions_revoked": revoked})
}

// handleAdminInsights — GET /api/v1/admin/insights — qué hay que mirar hoy en
// esta organización.
//
// La portada de Sytue Admin enseñaba recuentos totales —usuarios, grupos— y
// las últimas filas de auditoría: informa de que la organización existe, no de
// que haya algo que hacer. Estos cinco números sí llevan a una acción concreta,
// y son la razón por la que existen: gente sin segundo factor, cuentas
// suspendidas que nadie cerró, correos sin verificar e invitaciones que
// caducaron esperando a alguien.
//
// Los calcula el SERVIDOR sobre la organización entera. Deducirlos de la primera
// página del directorio —cincuenta filas— daría un «3 sin MFA» que en una
// organización de veinte mil personas es una respuesta inventada.
func (s *Server) handleAdminInsights(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in, err := s.store.OrgInsightsFor(r.Context(), org.ID, time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"without_mfa":         in.SinMFA,
		"suspended":           in.Suspendidos,
		"unverified_email":    in.SinVerificar,
		"invitations_expired": in.InvitacionesCaducadas,
		"invitations_pending": in.InvitacionesVivas,
	})
}
