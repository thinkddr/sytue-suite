// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

const eventCols = `id, calendar_id, object_id, uid, summary, description, location, organizer,
	dtstart, dtend, tzid, all_day, status, classification, transp, color, conference_url,
	rrule, rdate, exdate, recurrence_end, recurrence_id, sequence_no, created_at, updated_at`

func scanEvent(row pgx.Row, t string) (domain.Event, error) {
	var e domain.Event
	var status, class, transp string
	if err := row.Scan(&e.ID, &e.CalendarID, &e.ObjectID, &e.UID, &e.Summary, &e.Description,
		&e.Location, &e.Organizer, &e.DTStart, &e.DTEnd, &e.TZID, &e.AllDay,
		&status, &class, &transp, &e.Color, &e.ConferenceURL, &e.RRule, &e.RDate, &e.ExDate, &e.RecurrenceEnd,
		&e.RecurrenceID, &e.SequenceNo, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return domain.Event{}, err
	}
	e.Tenant = t
	e.Status = domain.EventStatus(status)
	e.Classification = domain.Classification(class)
	e.Transparency = domain.Transparency(transp)
	return e, nil
}

// CreateEvent crea un objeto CalDAV con su evento maestro en una sola
// transaccion, devolviendo el evento y el objeto (con su ETag). Avanza
// ctag/sync_token del calendario.
func (s *Store) CreateEvent(ctx context.Context, in domain.EventInput) (domain.Event, domain.CalendarObject, error) {
	t := tenancy.MustFrom(ctx)
	if in.UID == "" || in.DTEnd.Before(in.DTStart) {
		return domain.Event{}, domain.CalendarObject{}, domain.ErrInvalidEvent
	}
	in = withDefaults(in)

	var ev domain.Event
	var obj domain.CalendarObject
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Calendario destino debe existir, estar vivo y ser editable (no una
		// suscripción ICS de solo lectura).
		if err := assertWritable(ctx, tx, t.Slug, in.CalendarID); err != nil {
			return err
		}

		orow := tx.QueryRow(ctx,
			`INSERT INTO calendar_objects (tenant, calendar_id, uid, resource_name, component, data, size_bytes, sequence_no)
			 VALUES ($1, $2, $3, $4, 'VEVENT', $5, $6, $7)
			 RETURNING `+objectCols,
			t.Slug, in.CalendarID, in.UID, in.ResourceName, in.ICalData, len(in.ICalData), in.SequenceNo)
		var e error
		obj, e = scanObject(orow, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		if e != nil {
			return e
		}

		ev, e = insertEventRow(ctx, tx, t.Slug, obj.ID, in)
		if e != nil {
			return e
		}
		if err := replaceAttendees(ctx, tx, t.Slug, ev.ID, in.Attendees); err != nil {
			return err
		}
		ownerSub, err := calendarOwnerSub(ctx, tx, t.Slug, in.CalendarID)
		if err != nil {
			return err
		}
		if err := replaceReminders(ctx, tx, t.Slug, ownerSub, ev, in.ICalData, time.Now()); err != nil {
			return err
		}
		if err := bumpCalendar(ctx, tx, t.Slug, in.CalendarID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domain.Event{}, domain.CalendarObject{}, err
	}
	ev.Attendees = in.Attendees
	return ev, obj, nil
}

// UpdateEvent reescribe el objeto CalDAV y su evento maestro por UID dentro de un
// calendario. Si expectETag no es uuid.Nil, exige coincidencia (If-Match).
func (s *Store) UpdateEvent(ctx context.Context, in domain.EventInput, expectETag uuid.UUID) (domain.Event, domain.CalendarObject, error) {
	t := tenancy.MustFrom(ctx)
	if in.UID == "" || in.DTEnd.Before(in.DTStart) {
		return domain.Event{}, domain.CalendarObject{}, domain.ErrInvalidEvent
	}
	in = withDefaults(in)

	var ev domain.Event
	var obj domain.CalendarObject
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Un calendario de suscripción (read_only) no admite edición de usuario.
		if err := assertWritable(ctx, tx, t.Slug, in.CalendarID); err != nil {
			return err
		}
		var objID uuid.UUID
		var curETag uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT id, etag FROM calendar_objects
			 WHERE tenant=$1 AND calendar_id=$2 AND uid=$3 AND deleted_at IS NULL`,
			t.Slug, in.CalendarID, in.UID).Scan(&objID, &curETag)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if expectETag != uuid.Nil && curETag != expectETag {
			return domain.ErrPreconditionFailed
		}

		orow := tx.QueryRow(ctx,
			`UPDATE calendar_objects
			 SET data=$3, size_bytes=$4, sequence_no=$5, etag=gen_random_uuid(), updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL
			 RETURNING `+objectCols,
			t.Slug, objID, in.ICalData, len(in.ICalData), in.SequenceNo)
		obj, err = scanObject(orow, t.Slug)
		if err != nil {
			return err
		}

		// Reemplaza el evento maestro (recurrence_id IS NULL) del objeto.
		if _, err := tx.Exec(ctx,
			`DELETE FROM events WHERE tenant=$1 AND object_id=$2 AND recurrence_id IS NULL`,
			t.Slug, objID); err != nil {
			return fmt.Errorf("store: limpiar evento maestro: %w", err)
		}
		ev, err = insertEventRow(ctx, tx, t.Slug, objID, in)
		if err != nil {
			return err
		}
		if err := replaceAttendees(ctx, tx, t.Slug, ev.ID, in.Attendees); err != nil {
			return err
		}
		ownerSub, err := calendarOwnerSub(ctx, tx, t.Slug, in.CalendarID)
		if err != nil {
			return err
		}
		if err := replaceReminders(ctx, tx, t.Slug, ownerSub, ev, in.ICalData, time.Now()); err != nil {
			return err
		}
		if err := bumpCalendar(ctx, tx, t.Slug, in.CalendarID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domain.Event{}, domain.CalendarObject{}, err
	}
	ev.Attendees = in.Attendees
	return ev, obj, nil
}

// insertEventRow inserta una fila de evento (maestro u override) en `events`.
func insertEventRow(ctx context.Context, tx pgx.Tx, tenant string, objectID uuid.UUID, in domain.EventInput) (domain.Event, error) {
	var calendarID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT calendar_id FROM calendar_objects WHERE tenant=$1 AND id=$2`,
		tenant, objectID).Scan(&calendarID); err != nil {
		return domain.Event{}, err
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO events (tenant, calendar_id, object_id, uid, summary, description, location, organizer,
			dtstart, dtend, tzid, all_day, status, classification, transp, color, conference_url,
			rrule, rdate, exdate, recurrence_end, recurrence_id, sequence_no)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
		 RETURNING `+eventCols,
		tenant, calendarID, objectID, in.UID, in.Summary, in.Description, in.Location, in.Organizer,
		in.DTStart.UTC(), in.DTEnd.UTC(), in.TZID, in.AllDay, string(in.Status), string(in.Classification),
		string(in.Transparency), domain.SanitizeColor(in.Color),
		domain.SanitizeConferenceURL(in.ConferenceURL),
		in.RRule, utcSlice(in.RDate), utcSlice(in.ExDate),
		in.RecurrenceEnd, in.RecurrenceID, in.SequenceNo)
	ev, err := scanEvent(row, tenant)
	if uniqueViolation(err) {
		return domain.Event{}, domain.ErrConflict
	}
	return ev, err
}

// GetEvent devuelve un evento maestro por ID, con sus asistentes.
func (s *Store) GetEvent(ctx context.Context, id uuid.UUID) (domain.Event, error) {
	t := tenancy.MustFrom(ctx)
	var ev domain.Event
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+eventCols+` FROM events WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id)
		var e error
		ev, e = scanEvent(row, t.Slug)
		if e != nil {
			return e
		}
		ev.Attendees, e = loadAttendees(ctx, tx, t.Slug, ev.ID)
		return e
	})
	if isNoRows(err) {
		return domain.Event{}, domain.ErrNotFound
	}
	return ev, err
}

// maxEventsPerRange acota cuantos eventos devuelve una consulta de rango: un
// rango arbitrariamente amplio no puede volcar el calendario entero en una
// respuesta. El orden estable (dtstart, id) hace la truncacion determinista;
// el cliente recupera lo que falta consultando ventanas mas cortas.
const maxEventsPerRange = 1000

// ListEventsInRange devuelve los eventos vivos de un calendario que pueden tener
// instancias en [from, to): puntuales que solapan, o series cuya
// recurrence_end (NULL=infinita) alcanza el inicio de la ventana. La expansion
// de recurrencias la hace el dominio (ExpandEvents). Devuelve como maximo
// maxEventsPerRange eventos, en orden estable por (dtstart, id).
func (s *Store) ListEventsInRange(ctx context.Context, calendarID uuid.UUID, from, to time.Time) ([]domain.Event, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Event
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+eventCols+` FROM events
			 WHERE tenant=$1 AND calendar_id=$2 AND deleted_at IS NULL
			   AND dtstart < $4
			   AND (
			        (rrule = '' AND cardinality(rdate) = 0 AND dtend > $3)
			     OR (rrule <> '' OR cardinality(rdate) > 0)
			   )
			   AND (recurrence_end IS NULL OR recurrence_end > $3)
			 ORDER BY dtstart, id
			 LIMIT $5`,
			t.Slug, calendarID, from.UTC(), to.UTC(), maxEventsPerRange)
		if err != nil {
			return fmt.Errorf("store: eventos por rango: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			ev, err := scanEvent(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, ev)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return attachAttendees(ctx, tx, t.Slug, out)
	})
	return out, err
}

// ListEventsForOwnerInRange agrega los eventos en rango de TODOS los calendarios
// vivos de un dueno (para free/busy del usuario). El filtrado por transparencia
// (TRANSPARENT/CANCELLED no cuentan como ocupado) lo aplica el dominio al fusionar
// (domain.FreeBusy), de modo que aqui se devuelven todos los candidatos del rango.
func (s *Store) ListEventsForOwnerInRange(ctx context.Context, ownerSub string, from, to time.Time) ([]domain.Event, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Event
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+eventQual(`e`)+` FROM events e
			 JOIN calendars c ON c.id = e.calendar_id AND c.tenant = e.tenant
			 WHERE e.tenant=$1 AND c.owner_sub=$2 AND e.deleted_at IS NULL AND c.deleted_at IS NULL
			   AND e.dtstart < $4
			   AND (e.recurrence_end IS NULL OR e.recurrence_end > $3)
			   AND (
			        (e.rrule = '' AND cardinality(e.rdate) = 0 AND e.dtend > $3)
			     OR (e.rrule <> '' OR cardinality(e.rdate) > 0)
			   )
			 ORDER BY e.dtstart`,
			t.Slug, ownerSub, from.UTC(), to.UTC())
		if err != nil {
			return fmt.Errorf("store: free/busy por dueno: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			ev, err := scanEvent(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, ev)
		}
		return rows.Err()
	})
	return out, err
}

// DeleteEvent borra (soft) el objeto CalDAV completo y sus eventos por UID en un
// calendario, validando ETag si se indica. Devuelve ErrNotFound si no existe.
func (s *Store) DeleteEvent(ctx context.Context, calendarID uuid.UUID, uid string, expectETag uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Un calendario de suscripción (read_only) no admite borrado de usuario.
		if err := assertWritable(ctx, tx, t.Slug, calendarID); err != nil {
			return err
		}
		var objID, curETag uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT id, etag FROM calendar_objects
			 WHERE tenant=$1 AND calendar_id=$2 AND uid=$3 AND deleted_at IS NULL`,
			t.Slug, calendarID, uid).Scan(&objID, &curETag)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if expectETag != uuid.Nil && curETag != expectETag {
			return domain.ErrPreconditionFailed
		}
		if _, err := tx.Exec(ctx,
			`UPDATE events SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND object_id=$2 AND deleted_at IS NULL`, t.Slug, objID); err != nil {
			return fmt.Errorf("store: borrar eventos del objeto: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE calendar_objects SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND id=$2`, t.Slug, objID); err != nil {
			return fmt.Errorf("store: borrar objeto: %w", err)
		}
		// El soft-delete no dispara el ON DELETE CASCADE: retira sus avisos.
		if err := deleteRemindersForObject(ctx, tx, t.Slug, objID); err != nil {
			return err
		}
		err = bumpCalendar(ctx, tx, t.Slug, calendarID)
		return err
	})
}

// DeleteEventByID borra (soft) el objeto que contiene el evento maestro dado.
func (s *Store) DeleteEventByID(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	var calendarID uuid.UUID
	var uid string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT calendar_id, uid FROM events WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id).Scan(&calendarID, &uid)
	})
	if isNoRows(err) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.DeleteEvent(ctx, calendarID, uid, uuid.Nil)
}

// assertWritable verifica que un calendario exista, esté vivo y NO sea de solo
// lectura (una suscripción ICS externa). Es la ÚNICA puerta de escritura de
// eventos de usuario (create/update/delete), por la que pasan tanto la API REST
// como el PUT/DELETE CalDAV: un calendario suscrito rechaza toda escritura con
// ErrReadOnly. El refresco interno de la suscripción NO pasa por aquí.
func assertWritable(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) error {
	var readOnly bool
	err := tx.QueryRow(ctx,
		`SELECT read_only FROM calendars WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
		tenant, id).Scan(&readOnly)
	if isNoRows(err) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if readOnly {
		return domain.ErrReadOnly
	}
	return nil
}

// withDefaults rellena valores por defecto de un EventInput.
func withDefaults(in domain.EventInput) domain.EventInput {
	if in.TZID == "" {
		in.TZID = "UTC"
	}
	if in.Status == "" {
		in.Status = domain.StatusConfirmed
	}
	if in.Classification == "" {
		in.Classification = domain.ClassPublic
	}
	if in.Transparency == "" {
		in.Transparency = domain.TranspOpaque
	}
	if in.ResourceName == "" {
		in.ResourceName = in.UID + ".ics"
	}
	return in
}

func utcSlice(ts []time.Time) []time.Time {
	if len(ts) == 0 {
		return []time.Time{}
	}
	out := make([]time.Time, len(ts))
	for i, t := range ts {
		out[i] = t.UTC()
	}
	return out
}

// eventQual devuelve la lista de columnas de evento cualificadas por alias.
func eventQual(alias string) string {
	cols := []string{
		"id", "calendar_id", "object_id", "uid", "summary", "description", "location", "organizer",
		"dtstart", "dtend", "tzid", "all_day", "status", "classification", "transp", "color", "conference_url",
		"rrule", "rdate", "exdate", "recurrence_end", "recurrence_id", "sequence_no", "created_at", "updated_at",
	}
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = alias + "." + c
	}
	return strings.Join(out, ", ")
}
