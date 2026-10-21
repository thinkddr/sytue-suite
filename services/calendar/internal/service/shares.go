// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// Compartir calendarios con ACL. La gestión de shares (crear/listar/revocar) es
// solo del DUEÑO (el store lo verifica). El acceso a un calendario compartido pasa
// por CalendarAccess (la verja): ver los eventos si dueño|reader|writer; escribir
// solo si dueño|writer (un reader recibe ErrReadOnly, como una suscripción ICS).

// ShareCalendar comparte un calendario del dueño con un destinatario por correo.
// Valida el permiso y el correo en el borde de confianza y normaliza el correo
// (minúsculas) para casarlo luego con el claim `email` del JWT. Rechaza compartir
// con uno mismo. Solo el dueño (el store devuelve ErrNotFound si no lo es).
func (s *Service) ShareCalendar(ctx context.Context, calendarID uuid.UUID, ownerSub, ownerEmail, granteeEmail string, perm domain.SharePermission) (domain.CalendarShare, error) {
	if !domain.ValidSharePermission(perm) {
		return domain.CalendarShare{}, domain.ErrInvalidShare
	}
	if !domain.ValidEmail(granteeEmail) {
		return domain.CalendarShare{}, domain.ErrInvalidShare
	}
	email := domain.NormalizeEmail(granteeEmail)
	if ownerEmail != "" && email == domain.NormalizeEmail(ownerEmail) {
		return domain.CalendarShare{}, domain.ErrShareSelf
	}
	sh, err := s.store.CreateCalendarShare(ctx, calendarID, ownerSub, email, perm)
	if err != nil {
		return domain.CalendarShare{}, err
	}
	s.publish(ctx, "calendar.share.created", map[string]any{
		"calendar_id": calendarID, "permission": perm,
	})
	return sh, nil
}

// ListCalendarShares lista los shares de un calendario del dueño. Solo el dueño.
func (s *Service) ListCalendarShares(ctx context.Context, calendarID uuid.UUID, ownerSub string) ([]domain.CalendarShare, error) {
	return s.store.ListCalendarShares(ctx, calendarID, ownerSub)
}

// RevokeCalendarShare revoca un share (corta el acceso del destinatario). Solo el
// dueño.
func (s *Service) RevokeCalendarShare(ctx context.Context, calendarID, shareID uuid.UUID, ownerSub string) error {
	if err := s.store.RevokeCalendarShare(ctx, calendarID, shareID, ownerSub); err != nil {
		return err
	}
	s.publish(ctx, "calendar.share.revoked", map[string]any{
		"calendar_id": calendarID, "share_id": shareID,
	})
	return nil
}

// SharedWithMe lista los calendarios compartidos CON el usuario (por su sub o su
// correo del JWT), con su dueño y permiso. Deny-by-default: sin shares, vacío.
func (s *Service) SharedWithMe(ctx context.Context, sub, email string) ([]store.SharedCalendar, error) {
	return s.store.ListCalendarsSharedWith(ctx, sub, email)
}

// CalendarAccess devuelve el permiso efectivo del usuario sobre un calendario
// (dueño→writer, share→su permiso, ninguno→ErrNotFound). Fino passthrough a la
// verja del store.
func (s *Service) CalendarAccess(ctx context.Context, sub, email string, calendarID uuid.UUID) (domain.SharePermission, error) {
	return s.store.CalendarAccess(ctx, sub, email, calendarID)
}

// assertShareWritable es la puerta de ESCRITURA sobre un calendario compartido:
// requiere que el usuario sea dueño o writer; un reader recibe ErrReadOnly (mapea a
// 403, igual que una suscripción ICS de solo lectura); sin acceso → ErrNotFound
// (404). Reúsa la verja CalendarAccess.
func (s *Service) assertShareWritable(ctx context.Context, sub, email string, calendarID uuid.UUID) error {
	perm, err := s.store.CalendarAccess(ctx, sub, email, calendarID)
	if err != nil {
		return err // 404 deny-by-default
	}
	if !domain.CanWrite(perm) {
		return domain.ErrReadOnly // reader → 403 (misma semántica que ICS)
	}
	return nil
}
