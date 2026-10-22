// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/alarm"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// replaceReminders reemplaza los recordatorios del objeto del evento por los que
// derivan de sus VALARM cuyo disparo aún es futuro. Se ejecuta DENTRO de la tx de
// escritura del evento (create/update), tras insertar la fila maestra. Best-effort:
// un VALARM ilegible se ignora (no debe impedir guardar el evento).
//
// ponytail: para series (RRULE) arma solo el disparo relativo al DTSTART maestro
// (la primera ocurrencia futura). Re-armar la siguiente ocurrencia tras disparar
// queda como follow-up; hoy un evento recurrente creado en el pasado no re-avisa.
func replaceReminders(ctx context.Context, tx pgx.Tx, tenant, ownerSub string, ev domain.Event, icalData string, now time.Time) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM calendar_reminders WHERE tenant=$1 AND object_id=$2`, tenant, ev.ObjectID); err != nil {
		return fmt.Errorf("store: limpiar recordatorios: %w", err)
	}
	if ev.Status == domain.StatusCancelled {
		return nil // evento cancelado: sin avisos
	}
	triggers, err := alarm.Triggers(icalData, ev.DTStart, ev.DTEnd)
	if err != nil {
		return nil //nolint:nilerr // iCal ilegible para VALARM no bloquea la escritura del evento
	}
	for _, tr := range triggers {
		at := tr.At.UTC()
		if !at.After(now) {
			continue // ya pasó: no re-disparamos avisos vencidos
		}
		action := tr.Action
		if action == "" {
			action = "DISPLAY"
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO calendar_reminders
			   (tenant, owner_sub, calendar_id, object_id, event_id, uid, summary, dtstart, trigger_at, action)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			tenant, ownerSub, ev.CalendarID, ev.ObjectID, ev.ID, ev.UID, ev.Summary, ev.DTStart.UTC(), at, action); err != nil {
			return fmt.Errorf("store: insertar recordatorio: %w", err)
		}
	}
	return nil
}

// deleteRemindersForObject borra los recordatorios de un objeto. El borrado de
// eventos es SOFT (deleted_at), que no dispara el ON DELETE CASCADE de la FK, así
// que hay que retirar sus avisos pendientes explícitamente.
func deleteRemindersForObject(ctx context.Context, tx pgx.Tx, tenant string, objectID uuid.UUID) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM calendar_reminders WHERE tenant=$1 AND object_id=$2`, tenant, objectID); err != nil {
		return fmt.Errorf("store: borrar recordatorios: %w", err)
	}
	return nil
}

// calendarOwnerSub resuelve el dueño (owner_sub, destinatario del aviso) de un
// calendario dentro de la tx.
func calendarOwnerSub(ctx context.Context, tx pgx.Tx, tenant string, calendarID uuid.UUID) (string, error) {
	var sub string
	err := tx.QueryRow(ctx,
		`SELECT owner_sub FROM calendars WHERE tenant=$1 AND id=$2`, tenant, calendarID).Scan(&sub)
	return sub, err
}

// DueReminder es un recordatorio pendiente listo para avisar.
type DueReminder struct {
	ID        uuid.UUID
	Tenant    string
	OwnerSub  string
	EventID   uuid.UUID
	UID       string
	Summary   string
	DTStart   time.Time
	TriggerAt time.Time
	Action    string
}

// dueLookback acota hacia atrás la ventana de recordatorios in-app: una ausencia
// larga no debe volcar un backlog ilimitado (el canal durable es el bus/email;
// el in-app muestra lo reciente). El keyset+limit ya acota cada respuesta.
const dueLookback = 24 * time.Hour

// ReminderCursor es el punto de continuación keyset de la cola in-app: la última
// (trigger_at, id) devuelta. Nil = primera página.
type ReminderCursor struct {
	TriggerAt time.Time
	ID        uuid.UUID
}

// ListDueReminders devuelve los recordatorios VENCIDOS (trigger_at<=now) y no
// descartados del usuario `ownerSub`, del más reciente al más antiguo, paginados
// por keyset (trigger_at, id) DESC. Aislado por tenant (RLS, InTenantTx) y por
// dueño (owner_sub). Una sola consulta indexada (sin N+1) por
// calendar_reminders_inapp_idx.
func (s *Store) ListDueReminders(ctx context.Context, ownerSub string, now time.Time, before *ReminderCursor, limit int) ([]DueReminder, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var beforeTS, beforeID any // NULL en la primera página
	if before != nil {
		beforeTS = before.TriggerAt.UTC()
		beforeID = before.ID
	}
	var out []DueReminder
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, tenant, owner_sub, event_id, uid, summary, dtstart, trigger_at, action
			   FROM calendar_reminders
			  WHERE tenant=$1 AND owner_sub=$2
			    AND dismissed_at IS NULL
			    AND trigger_at <= $3 AND trigger_at > $4
			    AND ($5::timestamptz IS NULL OR (trigger_at, id) < ($5, $6))
			  ORDER BY trigger_at DESC, id DESC
			  LIMIT $7`,
			t.Slug, ownerSub, now.UTC(), now.UTC().Add(-dueLookback), beforeTS, beforeID, limit)
		if err != nil {
			return fmt.Errorf("store: leer recordatorios in-app: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var r DueReminder
			if err := rows.Scan(&r.ID, &r.Tenant, &r.OwnerSub, &r.EventID, &r.UID,
				&r.Summary, &r.DTStart, &r.TriggerAt, &r.Action); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// DismissReminder marca como descartado un recordatorio in-app del usuario. Es
// idempotente (re-descartar conserva la primera marca) y devuelve ErrNotFound si
// el recordatorio no existe para ese dueño en el tenant.
func (s *Store) DismissReminder(ctx context.Context, ownerSub string, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx,
			`UPDATE calendar_reminders SET dismissed_at = COALESCE(dismissed_at, now())
			  WHERE tenant=$1 AND owner_sub=$2 AND id=$3`,
			t.Slug, ownerSub, id)
		if err != nil {
			return fmt.Errorf("store: descartar recordatorio: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ClaimDueReminders reclama hasta `limit` recordatorios cuyo disparo ya venció y
// aún no se avisaron, publica cada uno con `publish` y los marca como disparados,
// todo en UNA transacción (at-least-once vía outbox: si el commit falla, ni se
// publican ni se marcan). Cross-tenant: usa `app.maintenance` (política PERMISSIVE),
// sin BYPASSRLS. FOR UPDATE SKIP LOCKED permite varias réplicas del worker sin
// doble entrega. Devuelve cuántos reclamó.
func (s *Store) ClaimDueReminders(ctx context.Context, now time.Time, limit int, publish func(context.Context, pg.Querier, DueReminder) error) (int, error) {
	var n int
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, tenant, owner_sub, event_id, uid, summary, dtstart, trigger_at, action
			   FROM calendar_reminders
			  WHERE fired_at IS NULL AND trigger_at <= $1
			  ORDER BY trigger_at
			  FOR UPDATE SKIP LOCKED
			  LIMIT $2`, now.UTC(), limit)
		if err != nil {
			return fmt.Errorf("store: leer recordatorios vencidos: %w", err)
		}
		var due []DueReminder
		for rows.Next() {
			var r DueReminder
			if err := rows.Scan(&r.ID, &r.Tenant, &r.OwnerSub, &r.EventID, &r.UID,
				&r.Summary, &r.DTStart, &r.TriggerAt, &r.Action); err != nil {
				rows.Close()
				return err
			}
			due = append(due, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		// La publicación va uno a uno —cada recordatorio es un evento distinto—, pero
		// el marcado NO: un UPDATE por fila son `limit` viajes de ida y vuelta dentro
		// de la transacción, y este worker despierta justo en las horas en punto, que
		// es cuando vence de golpe todo lo que la gente programó a las 9:00. Se
		// acumulan los ids y se marcan de una vez.
		ids := make([]uuid.UUID, 0, len(due))
		for _, r := range due {
			if err := publish(ctx, tx, r); err != nil {
				return err
			}
			ids = append(ids, r.ID)
		}
		if len(ids) == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE calendar_reminders SET fired_at=$1 WHERE id = ANY($2)`,
			now.UTC(), ids); err != nil {
			return err
		}
		n = len(ids)
		return nil
	})
	return n, err
}
