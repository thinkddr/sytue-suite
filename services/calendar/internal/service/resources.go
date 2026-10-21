// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// Recursos reservables (salas, equipos) — patrón resource-calendar. El servicio
// orquesta el store (que garantiza la atomicidad anti doble-reserva con un bloqueo
// de fila) y deriva la recurrence_end de la RRULE (como en un evento normal) para
// acotar la comprobación de solape de reservas recurrentes. La autorización de rol
// (admin para dar de alta; gestor para editar) la fija la capa HTTP con los claims.

// ListResources lista los recursos activos del tenant; con withAvailability, adjunta
// los periodos ocupados de cada uno en [from, to) (free/busy de su calendario).
func (s *Service) ListResources(ctx context.Context, from, to time.Time, withAvailability bool) ([]store.ResourceAvailability, error) {
	if withAvailability && !to.After(from) {
		return nil, domain.ErrInvalidRange
	}
	if withAvailability && to.Sub(from) > s.cfg.MaxRangeWindow {
		return nil, domain.ErrInvalidRange
	}
	return s.store.ListResources(ctx, from, to, withAvailability)
}

// GetResource devuelve un recurso por ID.
func (s *Service) GetResource(ctx context.Context, id uuid.UUID) (domain.Resource, error) {
	return s.store.GetResource(ctx, id)
}

// CreateResource da de alta un recurso (y su calendario de reservas). La verja de rol
// (solo admin/owner) la aplica la capa HTTP antes de llamar.
func (s *Service) CreateResource(ctx context.Context, nr store.NewResource) (domain.Resource, error) {
	r, err := s.store.CreateResource(ctx, nr)
	if err != nil {
		return domain.Resource{}, err
	}
	s.publish(ctx, "calendar.resource.created", map[string]any{"resource_id": r.ID, "kind": r.Kind})
	return r, nil
}

// UpdateResource edita un recurso (solo su gestor o un admin del tenant).
func (s *Service) UpdateResource(ctx context.Context, id uuid.UUID, sub string, isAdmin bool, p store.ResourcePatch) (domain.Resource, error) {
	if p.Email != nil && *p.Email != "" && !domain.ValidEmail(*p.Email) {
		return domain.Resource{}, domain.ErrInvalidResource
	}
	if p.Capacity != nil && *p.Capacity < 0 {
		return domain.Resource{}, domain.ErrInvalidResource
	}
	return s.store.UpdateResource(ctx, id, sub, isAdmin, p)
}

// DeleteResource desactiva un recurso (solo su gestor o un admin del tenant).
func (s *Service) DeleteResource(ctx context.Context, id uuid.UUID, sub string, isAdmin bool) error {
	if err := s.store.DeleteResource(ctx, id, sub, isAdmin); err != nil {
		return err
	}
	s.publish(ctx, "calendar.resource.deleted", map[string]any{"resource_id": id})
	return nil
}

// BookingSpec son los datos de una reserva desde la API: el UID del evento origen (que
// enlaza reserva↔evento) más su programación (rango + recurrencia + zona).
type BookingSpec struct {
	SourceUID string
	Summary   string
	Organizer string
	DTStart   time.Time
	DTEnd     time.Time
	TZID      string
	AllDay    bool
	RRule     string
	RDate     []time.Time
	ExDate    []time.Time
}

// BookResource reserva un recurso para el evento dado. Comprueba la disponibilidad de
// forma ATÓMICA (el store toma el bloqueo de fila del recurso) y devuelve el estado:
// ACCEPTED (libre + auto-accept), DECLINED (ocupado) o TENTATIVE (libre pero requiere
// aprobación manual). Cualquier usuario autenticado del tenant puede reservar un
// recurso activo; el `organizer` (por defecto, su email) queda como quien reservó.
func (s *Service) BookResource(ctx context.Context, resourceID uuid.UUID, spec BookingSpec) (store.BookingResult, error) {
	if spec.SourceUID == "" || spec.DTStart.IsZero() || spec.DTEnd.IsZero() {
		return store.BookingResult{}, domain.ErrInvalidEvent
	}
	if spec.DTEnd.Before(spec.DTStart) {
		return store.BookingResult{}, domain.ErrInvalidRange
	}
	b := store.ResourceBooking{
		SourceUID: spec.SourceUID,
		Summary:   spec.Summary,
		Organizer: cleanAddr(spec.Organizer),
		DTStart:   spec.DTStart.UTC(),
		DTEnd:     spec.DTEnd.UTC(),
		TZID:      orDefault(spec.TZID, "UTC"),
		AllDay:    spec.AllDay,
		RDate:     spec.RDate,
		ExDate:    spec.ExDate,
	}
	if spec.RRule != "" {
		rule, err := domain.ParseRRule(spec.RRule)
		if err != nil {
			return store.BookingResult{}, err
		}
		b.RRule = rule.String()
		loc := domain.EventLocation(b.TZID, b.AllDay)
		b.RecurrenceEnd = rule.RecurrenceEnd(b.DTStart.In(loc), b.DTEnd.Sub(b.DTStart))
	}
	res, err := s.store.BookResource(ctx, resourceID, b)
	if err != nil {
		return store.BookingResult{}, err
	}
	s.publish(ctx, "calendar.resource.booked", map[string]any{
		"resource_id": resourceID, "source_uid": spec.SourceUID, "status": res.Status,
	})
	return res, nil
}

// ReleaseResource libera la reserva del (evento, recurso). Autorizado para quien
// reservó, un gestor del recurso o un admin del tenant.
func (s *Service) ReleaseResource(ctx context.Context, sub, email string, isAdmin bool, resourceID uuid.UUID, sourceUID string) error {
	if err := s.store.ReleaseResourceBooking(ctx, resourceID, sourceUID, sub, email, isAdmin); err != nil {
		return err
	}
	s.publish(ctx, "calendar.resource.released", map[string]any{
		"resource_id": resourceID, "source_uid": sourceUID,
	})
	return nil
}
