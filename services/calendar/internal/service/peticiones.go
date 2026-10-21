// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"cmp"
	"context"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// «Pedir acceso de edición» de un calendario compartido (platform/pkg/peticiones):
// quien lo tiene de lector se lo pide al dueño, que lo concede con un clic desde el
// sitio al que lleva el aviso. Al conceder, el aviso `calendar.access.granted` le
// quita la franja a quien lo pidió por el flujo del Hub.

// SetAvisador cablea el aviso (Hub y correo) de las peticiones.
func (s *Service) SetAvisador(a peticiones.Avisador) { s.avisador = a }

// PedirEdicion deja la petición de `p` (sub, nombre y correo de su token) y avisa al
// dueño la primera vez. Solo un lector: al dueño o a un editor no le hace falta
// (yaPuede, sin petición) y a quien no ve el calendario, ErrNotFound.
func (s *Service) PedirEdicion(ctx context.Context, calendarID uuid.UUID, p peticiones.Peticion, token string) (yaPuede bool, err error) {
	perm, err := s.store.CalendarAccess(ctx, p.Sujeto, p.Correo, calendarID)
	if err != nil {
		return false, err
	}
	if domain.CanWrite(perm) {
		return true, nil
	}
	nueva, dueno, err := s.store.PedirEdicion(ctx, calendarID, p)
	if err != nil || !nueva {
		return false, err
	}
	// La petición ya está guardada: sin el nombre del calendario el aviso sale sin él.
	nombre := ""
	if cal, e := s.store.GetCalendar(ctx, calendarID); e == nil {
		nombre = cmp.Or(cal.DisplayName, cal.Name)
	}
	quien := p.Quien()
	s.avisador.Avisar(ctx, peticiones.Aviso{
		Tenant: tenancy.MustFrom(ctx).Slug, Para: dueno, Source: "calendar", Type: "calendar.access.requested",
		Titulo: quien + " asks to edit “" + nombre + "”", URL: hubnotify.EnlaceCalendario(s.cfg.PublicURL, tenancy.MustFrom(ctx).Slug, calendarID.String()),
		Recurso: calendarID.String(), Params: map[string]string{"actor": quien, "name": nombre}, Token: token,
		Asunto: quien + " asks to edit “" + nombre + "”",
		Cuerpo: quien + " is asking for permission to edit your calendar “" + nombre + "”. Open it to grant it with one click:",
	})
	return false, nil
}

// PeticionesEdicion son las pendientes de un calendario. Solo el dueño.
func (s *Service) PeticionesEdicion(ctx context.Context, calendarID uuid.UUID, ownerSub string) ([]peticiones.Peticion, error) {
	return s.store.PeticionesEdicion(ctx, calendarID, ownerSub)
}

// ResolverPeticion concede (de lector a editor) o rechaza. Solo el dueño, y solo a quien
// lo pidió. Al conceder avisa a quien lo pidió: es lo que le desbloquea la franja.
func (s *Service) ResolverPeticion(ctx context.Context, calendarID uuid.UUID, ownerSub, actor, sujeto string, concede bool) error {
	if err := s.store.ResolverPeticionEdicion(ctx, calendarID, ownerSub, sujeto, concede); err != nil {
		return err
	}
	if !concede {
		return nil
	}
	nombre := ""
	if cal, err := s.store.GetCalendar(ctx, calendarID); err == nil {
		nombre = cmp.Or(cal.DisplayName, cal.Name)
	}
	s.avisador.Avisar(ctx, peticiones.Aviso{
		Tenant: tenancy.MustFrom(ctx).Slug, Para: sujeto, Source: "calendar", Type: "calendar.access.granted",
		Titulo: "You can now edit “" + nombre + "”", URL: hubnotify.EnlaceCalendario(s.cfg.PublicURL, tenancy.MustFrom(ctx).Slug, calendarID.String()),
		Recurso: calendarID.String(), Params: map[string]string{"actor": actor, "name": nombre},
	})
	return nil
}
