// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// BAJA Y RESTAURACIÓN de una cuenta desde Sytue Admin.
//
// Lo que había: desde la consola solo se podía SUSPENDER. La baja de verdad
// (`status='deleted'`) la escribía únicamente el desaprovisionamiento SCIM, así
// que una organización sin IdP corporativo —la mayoría— no tenía forma de dar
// de baja a nadie: la cuenta se quedaba suspendida para siempre, contando como
// asiento y apareciendo en el directorio. Y cuando la baja sí ocurría era de un
// solo sentido: reactivar contesta 409 «This account has been closed» y ahí se
// acababa el camino.
//
// Tres decisiones que gobiernan este fichero:
//
//   - LA BAJA PIDE UN MOTIVO. No por burocracia: es lo que lee dentro de un mes
//     quien tiene que decidir si la restaura.
//   - RESTAURAR DEVUELVE LOS DATOS, NO EL ACCESO. La cuenta vuelve como
//     SUSPENDIDA. Reactivar es un segundo gesto, con su propia auditoría, y así
//     una restauración por equivocación no deja a nadie entrando esa tarde.
//   - LA VENTANA ES REAL. Pasados los días de retención la restauración se
//     niega, porque una recuperación sin caducidad es decir que la baja no
//     significaba nada.
package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// retencionBaja es cuánto se puede deshacer una baja. Treinta días: lo que
// tarda en aparecer el «pues resulta que vuelve» y lo que cabe defender ante
// quien pregunte por qué seguían ahí los datos de alguien que se fue.
//
// ponytail: constante, no ajuste por organización. Cuando una organización
// pida la suya, vive en `organizations` y se lee aquí; hoy nadie la ha pedido y
// un ajuste sin quien lo use es una pantalla más que mantener.
const retencionBaja = 30 * 24 * time.Hour

// handleAdminCloseUser — POST /api/v1/admin/users/{id}/close.
func (s *Server) handleAdminCloseUser(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, id, mem, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	// Los mismos guardas que cortar el acceso: ni sobre uno mismo ni un admin
	// sobre un owner. Dar de baja es más fuerte que suspender, así que aflojar
	// aquí sería una vía para hacer por la puerta de al lado lo que suspender no
	// deja.
	if e := adminMayManage(c.Subject, id, c.Roles, mem.Role); e != nil {
		httpx.Fail(w, r, e)
		return
	}
	var cuerpo struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	motivo := validation.TruncateRunes(strings.TrimSpace(cuerpo.Reason), 500)
	if motivo == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"Say why this account is being closed: it is what whoever decides on restoring it will read."))
		return
	}

	ctx := r.Context()
	var cerrada bool
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		// Anti-lockout: dar de baja al último owner deja la organización sin
		// nadie que pueda administrarla, y a diferencia de quitar la membresía
		// esto además apaga la cuenta. El recuento va DENTRO de la transacción,
		// contra el estado de ahora.
		if mem.Role == domain.RoleOwner {
			n, e := s.store.CountOrgOwners(ctx, q, org.ID, id)
			if e != nil {
				return e
			}
			if n <= 1 {
				return errLastOwner
			}
		}
		ok, e := s.store.CloseUser(ctx, q, id, c.Subject, motivo)
		cerrada = ok
		return e
	})
	switch {
	case errors.Is(err, errLastOwner):
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
			"This is the only owner of the organisation: name another owner first."))
		return
	case err != nil:
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	case !cerrada:
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("This account was already closed."))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.user.closed", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"id": id, "status": string(domain.StatusDeleted),
		// Hasta cuándo se puede deshacer, dicho en la misma respuesta que la
		// hace: es el dato que hay que apuntar, y buscarlo después es peor.
		"restorable_until": time.Now().Add(retencionBaja),
	})
}

// handleAdminRestoreUser — POST /api/v1/admin/users/{id}/restore.
func (s *Server) handleAdminRestoreUser(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, id, _, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	// Sin adminMayManage: restaurar no CORTA nada, y el caso de «un admin
	// restaura a un owner» es justo el que hay que poder hacer cuando se dio de
	// baja al owner por error. Lo que sigue en pie es el aislamiento por
	// organización de adminTargetInOrg y el permiso users.manage de la ruta.
	err := s.store.RestoreUser(r.Context(), nil, id, time.Now().Add(-retencionBaja))
	if errors.Is(err, store.ErrNoRecuperable) {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
			"This account cannot be restored: either it is not closed, or its recovery window has passed."))
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.user.restored", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"id": id, "status": string(domain.StatusSuspended),
		// Lo dice la respuesta para que nadie dé por hecho que ya puede entrar:
		// restaurar devuelve los datos, reactivar devuelve el acceso.
		"note": "restored as suspended; reactivate it to give access back",
	})
}

// handleAdminListClosedUsers — GET /api/v1/admin/users/closed — las bajas que
// todavía se pueden deshacer.
//
// Sin este listado la restauración sería inalcanzable en la práctica: el
// directorio no enseña las cuentas dadas de baja, así que habría que conocer de
// memoria el uuid de quien se quiere recuperar.
func (s *Server) handleAdminListClosedUsers(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	desde := time.Now().Add(-retencionBaja)
	lista, err := s.store.ClosedUsersInOrg(r.Context(), org.ID, desde)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	out := make([]map[string]any, 0, len(lista))
	for _, c := range lista {
		out = append(out, map[string]any{
			"id": c.ID, "email": c.Email, "name": c.Name,
			"closed_at": c.ClosedAt, "closed_by": c.ClosedBy, "reason": c.Reason,
			"restorable_until": c.ClosedAt.Add(retencionBaja),
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"users": out, "retention_days": int(retencionBaja.Hours() / 24),
	})
}
