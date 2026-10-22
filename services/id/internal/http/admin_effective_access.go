// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// El ALCANCE EFECTIVO de una persona: qué puede hacer de verdad, resuelto.
//
// Sytue Admin enseñaba las piezas por separado —el rol de la membresía en el
// directorio, los roles personalizados en su pantalla, PIM en la suya y los
// grupos en otra—, y la suma la hacía a ojo quien administra. Con tres fuentes
// que se solapan y una que caduca sola, esa suma se falla: es la diferencia
// entre creer que a alguien se le retiró un permiso y que siga teniéndolo por
// otro sitio.
//
// Esto no concede nada: es una LECTURA que responde con la misma regla que
// aplica el enforcement (domain.AlcanceEfectivo, la misma que EffectivePerms y
// HasPermission), para que lo que se enseña y lo que se autoriza no puedan
// divergir.
package http

import (
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// handleAdminUserEffectiveAccess — GET /api/v1/admin/users/{id}/effective-access.
func (s *Server) handleAdminUserEffectiveAccess(w http.ResponseWriter, r *http.Request) {
	org, id, mem, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	ctx := r.Context()
	ahora := time.Now()

	roles, err := s.store.CustomRolesForUser(ctx, org.ID, id)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// Todas las activaciones del usuario, no solo las «active»: una pendiente de
	// aprobación es parte de la respuesta —dice que hay una petición esperando—
	// aunque no conceda nada. El filtro de qué CONCEDE lo hace el dominio.
	activaciones, err := s.store.ListActivations(ctx, org.ID, id, "")
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	grupos, err := s.store.GroupsForUser(ctx, org.ID, id)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}

	elevaciones := make([]domain.ElevacionViva, 0, len(activaciones))
	pendientes := make([]map[string]any, 0, 2)
	for _, a := range activaciones {
		estado := a.Status
		expira := time.Time{}
		if a.ExpiresAt != nil {
			expira = *a.ExpiresAt
			estado = domain.EffectiveStatus(a.Status, expira, ahora)
		}
		if estado == domain.ActivationPending {
			pendientes = append(pendientes, map[string]any{
				"id": a.ID, "role": a.RoleName, "requested_at": a.RequestedAt,
				"justification": a.Justification,
			})
		}
		elevaciones = append(elevaciones, domain.ElevacionViva{
			Nombre: a.RoleName, Perms: a.Perms, Builtin: a.RoleID == "",
			ExpiresAt: expira, Status: a.Status,
		})
	}

	concesiones := domain.AlcanceEfectivo(string(mem.Role), roles, elevaciones, ahora)
	rolesSalida := make([]map[string]any, 0, len(roles))
	for _, rol := range roles {
		rolesSalida = append(rolesSalida, map[string]any{
			"id": rol.ID, "name": rol.Name, "permissions": rol.Permissions,
		})
	}
	if grupos == nil {
		grupos = []string{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"user_id": id,
		"role":    string(mem.Role),
		// La respuesta a «¿qué puede hacer?», con el porqué de cada línea: sin la
		// fuente no se sabe qué hay que tocar para quitarlo.
		"permissions": concesiones,
		// Y las piezas de las que sale, para poder ir a arreglarlas.
		"custom_roles": rolesSalida,
		"groups":       grupos,
		// Elevaciones esperando a que alguien las apruebe: no conceden todavía,
		// pero forman parte de lo que hay que mirar antes de decidir.
		"pending_elevations": pendientes,
		"as_of":              ahora,
	})
}
