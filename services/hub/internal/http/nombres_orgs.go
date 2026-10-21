// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"context"
	"slices"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// nombresOrgs da el nombre visible de unas organizaciones por su slug. Lo que no
// sabe no sale.
type nombresOrgs interface {
	Nombres(ctx context.Context, slugs []string) map[string]string
}

// conNombres pone a cada aviso de fuera el nombre visible de la organización que lo
// mandó (decisión del dueño, 17/10/2026). Es la verja de Sytue ID: solo se pregunta
// por las organizaciones de los avisos que la persona ya tiene en su campana —una
// invitación, o lo que vino después de aceptarla—, nunca por una que ella elija. Sin
// nombre, la campana sigue con el identificador.
func (h *Handlers) conNombres(ctx context.Context, items []domain.Notification) {
	if h.orgs == nil {
		return
	}
	var slugs []string
	for _, n := range items {
		if n.Organization != "" && !slices.Contains(slugs, n.Organization) {
			slugs = append(slugs, n.Organization)
		}
	}
	if len(slugs) == 0 {
		return
	}
	nombres := h.orgs.Nombres(ctx, slugs)
	for i := range items {
		items[i].OrganizationName = nombres[items[i].Organization]
	}
}
