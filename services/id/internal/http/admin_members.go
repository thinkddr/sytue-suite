// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Gestión de la MEMBRESÍA de un usuario en la organización activa desde Sytue
// Console (contrapartida admin de las acciones de /account): cambiar su rol,
// quitarlo de la org y disparar un reseteo de contraseña por email. Comparten el
// mismo anclaje de seguridad que admin_users.go:
//
//   - requireRole ya exige owner/admin en la org activa (deny-by-default).
//   - adminTargetInOrg deriva la org del claim (c.Tenant), NUNCA del path, y exige
//     que el objetivo sea miembro de ESA org (anti-IDOR / aislamiento por tenant).
//   - adminMayManage añade las invariantes de "corte": no sobre uno mismo (para eso
//     está /account, y evita autodegradarse) y a un owner solo lo toca otro owner.
//   - Guard del "último admin": nunca dejar la org sin ningún owner/admin.
//
// Quitar a un miembro y degradarlo revocan sus sesiones (el acceso ya emitido no
// sobrevive a la pérdida de privilegio).
package http

import (
	"context"
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	plog "git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// errLastAdmin señala que la acción dejaría a la organización sin ningún
// administrador (owner/admin). Se decide dentro de la tx (sobre el recuento en
// vivo) y se mapea a 409.
var errLastAdmin = errors.New("id: la organización se quedaría sin administradores")

// errLastOwner señala que la acción dejaría a la organización SIN NINGÚN owner. Es un
// invariante más fuerte que el del último admin (owner|admin): una org SIEMPRE debe
// conservar ≥1 owner. Se decide dentro de la tx (recuento en vivo) y se mapea a 409.
var errLastOwner = errors.New("id: la organización se quedaría sin owner")

// wouldLeaveOrgWithoutOwner es la decisión pura del guard «≥1 owner»: cambiar el rol
// del objetivo (o quitarlo, next="") lo saca del rol owner y era el ÚLTIMO. ownerCount
// es el recuento EN VIVO de owners de la org (incluye al objetivo).
func wouldLeaveOrgWithoutOwner(current, next domain.Role, ownerCount int) bool {
	return current == domain.RoleOwner && next != domain.RoleOwner && ownerCount <= 1
}

// roleRank ordena los roles por privilegio para detectar degradaciones (una
// bajada de privilegio revoca sesiones). owner > admin > member/billing.
func roleRank(r domain.Role) int {
	switch r {
	case domain.RoleOwner:
		return 3
	case domain.RoleAdmin:
		return 2
	default:
		return 1
	}
}

// isAdminRole indica si el rol pertenece al conjunto de administración (owner o
// admin), el que el guard del último admin protege de vaciarse.
func isAdminRole(r domain.Role) bool { return r == domain.RoleOwner || r == domain.RoleAdmin }

type setRoleRequest struct {
	Role string `json:"role"`
}

// handleAdminSetMemberRole — POST /api/v1/admin/users/{id}/role — cambia el rol del
// usuario en la org activa. Un admin no puede conceder owner (solo un owner). No se
// puede degradar al último administrador. Una degradación (bajada de privilegio)
// revoca las sesiones del usuario para que su token elevado no sobreviva.
//
// aplicarRol cambia el rol de un miembro con los dos guardas anti-lockout, en
// UNA transacción: los recuentos de owners y de administradores se hacen dentro,
// contra el estado en vivo, o dos cambios simultáneos podrían dejar la
// organización sin nadie que la administre cada uno creyendo que quedaba otro.
//
// Extraída del handler para que el cambio EN LOTE use exactamente los mismos
// guardas. Una segunda implementación de esto acabaría divergiendo, y el día que
// divergiera nadie se enteraría hasta quedarse fuera de su propia organización.
//
// Devuelve cuántas sesiones se cerraron: degradar a alguien tiene que cortarle
// el acceso que ya tenía emitido, no solo el siguiente.
//
//nolint:gocognit,gocyclo // cambio de rol: verjas anti-lockout (último owner, autodegradación, rol personalizado) sobre la misma tx.
func (s *Server) aplicarRol(ctx context.Context, orgID, userID string, actual, nuevo domain.Role) (int, error) {
	demotion := roleRank(nuevo) < roleRank(actual)
	var revoked int
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		// Anti-lockout «≥1 owner»: degradar a un owner (a cualquier rol no-owner) solo
		// procede si NO es el último owner (recuento en vivo). Se comprueba antes que el
		// guard del último admin: owner→member dispara ambos; owner→admin, solo este.
		if actual == domain.RoleOwner && nuevo != domain.RoleOwner {
			n, e := s.store.CountOrgOwners(ctx, q, orgID, userID)
			if e != nil {
				return e
			}
			if wouldLeaveOrgWithoutOwner(actual, nuevo, n) {
				return errLastOwner
			}
		}
		// Si el cambio saca al usuario del conjunto de administración, comprobar sobre
		// el recuento EN VIVO que no es el último administrador.
		if isAdminRole(actual) && !isAdminRole(nuevo) {
			n, e := s.store.CountOrgAdmins(ctx, q, orgID, userID)
			if e != nil {
				return e
			}
			if n <= 1 {
				return errLastAdmin
			}
		}
		if e := s.store.UpsertMembership(ctx, q, domain.Membership{
			OrgID: orgID, UserID: userID, Role: nuevo,
		}); e != nil {
			return e
		}
		if demotion {
			var e error
			revoked, e = s.store.RevokeAllUserSessions(ctx, q, userID)
			return e
		}
		return nil
	})
	return revoked, err
}

func (s *Server) handleAdminSetMemberRole(w http.ResponseWriter, r *http.Request) {
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
	var req setRoleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	newRole := domain.Role(req.Role)
	if !newRole.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid role (owner|admin|member|billing)."))
		return
	}
	// Solo un owner concede owner (evita que un admin escale a otro a su nivel/superior).
	if newRole == domain.RoleOwner && !hasAnyRole(c.Roles, domain.RoleOwner) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Only an owner can assign the owner role."))
		return
	}
	if newRole == mem.Role {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "role": newRole, "sessions_revoked": 0})
		return
	}
	revoked, err := s.aplicarRol(r.Context(), org.ID, id, mem.Role, newRole)
	ctx := r.Context()
	if err != nil {
		if errors.Is(err, errLastOwner) {
			httpx.Fail(w, r, httpx.ErrConflict.WithMessage("You cannot demote the last owner of the organisation."))
			return
		}
		if errors.Is(err, errLastAdmin) {
			httpx.Fail(w, r, httpx.ErrConflict.WithMessage("You cannot leave the organisation without any administrator."))
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.member.role_changed", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "role": newRole, "sessions_revoked": revoked})
}

// handleAdminRemoveMember — DELETE /api/v1/admin/users/{id}/membership — quita al
// usuario de la org activa y revoca sus sesiones (el acceso ya emitido cae con la
// membresía). No borra la cuenta (puede pertenecer a otras orgs). No se puede
// quitar al último administrador ni a uno mismo (adminMayManage).
func (s *Server) handleAdminRemoveMember(w http.ResponseWriter, r *http.Request) {
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
		// Anti-lockout «≥1 owner»: no se puede quitar al último owner (aunque queden
		// otros admins). Guard más fuerte que el del último admin, que va después.
		if mem.Role == domain.RoleOwner {
			n, e := s.store.CountOrgOwners(ctx, q, org.ID, id)
			if e != nil {
				return e
			}
			if wouldLeaveOrgWithoutOwner(mem.Role, "", n) {
				return errLastOwner
			}
		}
		if isAdminRole(mem.Role) {
			n, e := s.store.CountOrgAdmins(ctx, q, org.ID, id)
			if e != nil {
				return e
			}
			if n <= 1 {
				return errLastAdmin
			}
		}
		if e := s.store.RemoveMembership(ctx, q, org.ID, id); e != nil {
			return e
		}
		// ponytail: revoca TODAS las sesiones del usuario, no solo las de esta org
		// (sessions.org_id existe); un usuario multi-org saldría de todas. Es el fallo
		// seguro (cortar de más) y espeja a suspend; acotar por org si algún día pesa.
		var e error
		revoked, e = s.store.RevokeAllUserSessions(ctx, q, id)
		return e
	})
	if err != nil {
		if errors.Is(err, errLastOwner) {
			httpx.Fail(w, r, httpx.ErrConflict.WithMessage("You cannot remove the last owner of the organisation."))
			return
		}
		if errors.Is(err, errLastAdmin) {
			httpx.Fail(w, r, httpx.ErrConflict.WithMessage("You cannot remove the last administrator of the organisation."))
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.member.removed", id, r))
	s.olvidarBuzonesEnElHub(ctx, org.ID, id)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "sessions_revoked": revoked})
}

// handleAdminTriggerReset — POST /api/v1/admin/users/{id}/reset-password — dispara
// el flujo de "olvidé mi contraseña" para el usuario: le envía por email un enlace
// de reseteo (firmado, un solo uso, corta vida) para que ELIJA su nueva contraseña.
// El admin no ve ni fija la contraseña (a diferencia de handleAdminSetPassword). Al
// completar el reseteo se revocan las sesiones (handleResetPasswordSubmit).
func (s *Server) handleAdminTriggerReset(w http.ResponseWriter, r *http.Request) {
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
	user, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if user.PasswordHash == "" {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("This account does not use a password (sign-in is SSO/MFA only)."))
		return
	}
	if err := s.sendResetLink(r.Context(), user); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "user.password.reset_link_sent", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "sent": true})
}

// olvidarBuzonesEnElHub: quien sale de una organización deja de leer, desde cualquier
// otra, los avisos a sus direcciones de ésta (ADR 0053). Su siguiente token ya no las
// trae (el claim `mailboxes` sale de sus membresías), y esto corta además el que
// tenga vivo, que dura unos minutos. Mejor esfuerzo: sin Hub, la baja sigue.
func (s *Server) olvidarBuzonesEnElHub(ctx context.Context, orgID, userID string) {
	hub := clienteHub(s.cfg)
	if hub == nil {
		return
	}
	buzones, err := s.store.ListOrgMailboxes(ctx, orgID, userID)
	if err != nil {
		return
	}
	for _, b := range buzones {
		if err := hub.OlvidarBuzon(ctx, b.Address); err != nil {
			plog.From(ctx).Warn("baja: el Hub no olvidó los avisos de una dirección", "error", err)
		}
	}
}
