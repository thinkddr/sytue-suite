// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
)

// Los avisos de SEGURIDAD de la propia cuenta: lo que alguien tiene que saber aunque
// no lo haya hecho él —y justo entonces más—. Van a la campana de la suite por la
// puerta interna del Hub, como los de cualquier producto.
//
// El título de aquí es el respaldo de un cliente que no conozca el tipo; cada cliente
// lo compone en su idioma con `notifications.kind.<tipo>`.
var titulosSeguridad = map[string]string{ //nolint:gosec // G101: son títulos de avisos, no credenciales
	"security.signin.new":       "Nuevo inicio de sesión en tu cuenta",
	"security.password.changed": "Se ha cambiado la contraseña de tu cuenta",
	"security.mfa.enabled":      "Se ha activado la verificación en dos pasos",
	"security.mfa.disabled":     "Se ha desactivado la verificación en dos pasos",
	"security.passkey.added":    "Se ha añadido una llave de acceso a tu cuenta",
	"security.passkey.removed":  "Se ha quitado una llave de acceso de tu cuenta",
}

// avisarSeguridad deja el aviso `tipo` en la campana de `sub`. `org` es la
// organización en la que ocurre: su slug (el `tenant` de un token) o su id (el de una
// sesión de navegador); vacía, la primera de la persona. El aviso es de la PERSONA y
// no de la organización (ADR 0053), así que la campana lo enseña entre desde la que
// entre. Mejor esfuerzo y sin esperar: sin Hub, el cambio ya está hecho y auditado.
func (s *Server) avisarSeguridad(ctx context.Context, org, sub, tipo string, params map[string]string) {
	if s.hub == nil || sub == "" {
		return
	}
	tenant := org
	if tenant == "" {
		if mems, err := s.store.ListMembershipsForUser(ctx, sub); err == nil && len(mems) > 0 {
			tenant = mems[0].OrgID
		}
	}
	if _, err := uuid.Parse(tenant); err == nil {
		o, err := s.store.GetOrgByID(ctx, tenant)
		if err != nil {
			return
		}
		tenant = o.Slug
	}
	if tenant == "" {
		return // sin organización no hay campana en la que dejarlo
	}
	s.hub.SendAsync(ctx, hubnotify.Notification{
		Tenant:  tenant,
		Subject: sub,
		Source:  "id",
		Type:    tipo,
		Level:   "warning",
		Title:   titulosSeguridad[tipo],
		URL:     s.cfg.Issuer + "/account#seguridad",
		Params:  params,
	})
}
