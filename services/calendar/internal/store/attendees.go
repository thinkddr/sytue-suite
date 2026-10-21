// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

const attendeeCols = `id, email, common_name, role, partstat, cutype, rsvp`

func scanAttendee(row pgx.Row) (domain.Attendee, error) {
	var a domain.Attendee
	var role, partstat string
	if err := row.Scan(&a.ID, &a.Email, &a.CommonName, &role, &partstat, &a.CUType, &a.RSVP); err != nil {
		return domain.Attendee{}, err
	}
	a.Role = domain.AttendeeRole(role)
	a.PartStat = domain.ParticipationStatus(partstat)
	return a, nil
}

// replaceAttendees reemplaza la lista de asistentes de un evento (en la tx).
func replaceAttendees(ctx context.Context, tx pgx.Tx, tenant string, eventID uuid.UUID, attendees []domain.Attendee) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM attendees WHERE tenant=$1 AND event_id=$2`, tenant, eventID); err != nil {
		return fmt.Errorf("store: limpiar asistentes: %w", err)
	}
	for _, a := range attendees {
		email := strings.TrimSpace(a.Email)
		if email == "" {
			continue
		}
		if a.Role == "" {
			a.Role = domain.RoleRequired
		}
		if a.PartStat == "" {
			a.PartStat = domain.PartNeedsAction
		}
		if a.CUType == "" {
			a.CUType = "INDIVIDUAL"
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO attendees (tenant, event_id, email, common_name, role, partstat, cutype, rsvp)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (tenant, event_id, lower(email)) DO UPDATE SET
				common_name = EXCLUDED.common_name,
				role        = EXCLUDED.role,
				partstat    = EXCLUDED.partstat,
				cutype      = EXCLUDED.cutype,
				rsvp        = EXCLUDED.rsvp,
				updated_at  = now()`,
			tenant, eventID, email, a.CommonName, string(a.Role), string(a.PartStat), a.CUType, a.RSVP); err != nil {
			return fmt.Errorf("store: insertar asistente: %w", err)
		}
	}
	return nil
}

// loadAttendees carga los asistentes de un evento (en la tx).
func loadAttendees(ctx context.Context, tx pgx.Tx, tenant string, eventID uuid.UUID) ([]domain.Attendee, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+attendeeCols+` FROM attendees WHERE tenant=$1 AND event_id=$2 ORDER BY lower(email)`,
		tenant, eventID)
	if err != nil {
		return nil, fmt.Errorf("store: leer asistentes: %w", err)
	}
	defer rows.Close()
	var out []domain.Attendee
	for rows.Next() {
		a, err := scanAttendee(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// attachAttendees carga los asistentes de un lote de eventos en una sola
// consulta (evita N+1) y los asigna a cada evento por ID.
func attachAttendees(ctx context.Context, tx pgx.Tx, tenant string, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(events))
	idx := make(map[uuid.UUID]int, len(events))
	for i, e := range events {
		ids[i] = e.ID
		idx[e.ID] = i
	}
	rows, err := tx.Query(ctx,
		`SELECT event_id, `+attendeeCols+` FROM attendees
		 WHERE tenant=$1 AND event_id = ANY($2)
		 ORDER BY lower(email)`, tenant, ids)
	if err != nil {
		return fmt.Errorf("store: leer asistentes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var eventID uuid.UUID
		var a domain.Attendee
		var role, partstat string
		if err := rows.Scan(&eventID, &a.ID, &a.Email, &a.CommonName, &role, &partstat, &a.CUType, &a.RSVP); err != nil {
			return err
		}
		a.Role = domain.AttendeeRole(role)
		a.PartStat = domain.ParticipationStatus(partstat)
		i, ok := idx[eventID]
		if !ok {
			continue
		}
		events[i].Attendees = append(events[i].Attendees, a)
	}
	return rows.Err()
}

// SetAttendeePartStat actualiza el PARTSTAT de un asistente por email en un
// evento (respuesta a invitacion / RSVP). Devuelve el asistente actualizado.
func (s *Store) SetAttendeePartStat(ctx context.Context, eventID uuid.UUID, email string, partstat domain.ParticipationStatus) (domain.Attendee, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidPartStat(partstat) {
		return domain.Attendee{}, domain.ErrInvalidEvent
	}
	var a domain.Attendee
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE attendees SET partstat=$3, updated_at=now()
			 WHERE tenant=$1 AND event_id=$2 AND lower(email)=lower($4)
			 RETURNING `+attendeeCols,
			t.Slug, eventID, string(partstat), strings.TrimSpace(email))
		var e error
		a, e = scanAttendee(row)
		return e
	})
	if isNoRows(err) {
		return domain.Attendee{}, domain.ErrNotFound
	}
	return a, err
}
