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

// Recursos reservables (salas, equipos) — patrón resource-calendar. Cada recurso
// tiene un calendario propio (backing calendar) con dueño sintético
// 'resource:<id>' donde viven sus reservas; reservar = crear un evento ahí. La
// disponibilidad del recurso es el free/busy de ese calendario (mismo motor).
//
// ANTI DOBLE-RESERVA ATÓMICO: BookResource toma un `SELECT ... FOR UPDATE` sobre la
// FILA del recurso antes de comprobar el solape e insertar la reserva. Todas las
// reservas de un mismo recurso se serializan por esa fila: dos reservas simultáneas
// de la misma sala/franja no pueden ganar ambas (la segunda, ya con el lock, ve la
// reserva de la primera y rechaza). Recursos distintos no contienden (lock por fila).
// Todo tenant-scoped por RLS (InTenantTx).

// resourceCalendarPrefix es el dueño sintético del backing calendar de un recurso.
// Ningún usuario tiene este sub, así el calendario del recurso nunca aparece en la
// lista de calendarios de nadie ni contamina su free/busy.
const resourceCalendarPrefix = "resource:"

// maxConflictWindow acota la ventana de comprobación de solape de una reserva
// RECURRENTE sin UNTIL/COUNT (serie infinita): se comprueban conflictos hasta un año
// por delante. ponytail: ceiling pragmático; ampliar si aparecen reservas perpetuas
// que deban validarse más lejos.
const maxConflictWindow = 366 * 24 * time.Hour

const resourceCols = `id, calendar_id, kind, name, capacity, location, email,
	auto_accept, managers, active, created_by, created_at, updated_at`

func scanResource(row pgx.Row, t string) (domain.Resource, error) {
	var r domain.Resource
	var kind string
	if err := row.Scan(&r.ID, &r.CalendarID, &kind, &r.Name, &r.Capacity, &r.Location,
		&r.Email, &r.AutoAccept, &r.Managers, &r.Active, &r.CreatedBy,
		&r.CreatedAt, &r.UpdatedAt); err != nil {
		return domain.Resource{}, err
	}
	r.Tenant = t
	r.Kind = domain.ResourceKind(kind)
	if r.Managers == nil {
		r.Managers = []string{}
	}
	return r, nil
}

// NewResource son los datos de alta de un recurso. Managers/CreatedBy son subs.
type NewResource struct {
	Kind       domain.ResourceKind
	Name       string
	Capacity   int
	Location   string
	Email      string
	AutoAccept bool
	Managers   []string
	CreatedBy  string
}

// CreateResource crea un recurso y su calendario de reservas (backing calendar) en
// una sola transacción. El calendario es del dueño sintético 'resource:<id>'.
func (s *Store) CreateResource(ctx context.Context, nr NewResource) (domain.Resource, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidResource(nr.Kind, nr.Name, nr.Capacity, nr.Email) {
		return domain.Resource{}, domain.ErrInvalidResource
	}
	if nr.Managers == nil {
		nr.Managers = []string{}
	}
	resourceID := uuid.New()
	var r domain.Resource
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var calendarID uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO calendars (tenant, owner_sub, name, display_name, timezone)
			 VALUES ($1, $2, 'bookings', $3, 'UTC')
			 RETURNING id`,
			t.Slug, resourceCalendarPrefix+resourceID.String(), strings.TrimSpace(nr.Name)).
			Scan(&calendarID); err != nil {
			return fmt.Errorf("store: crear calendario de recurso: %w", err)
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO calendar_resources
			   (id, tenant, calendar_id, kind, name, capacity, location, email, auto_accept, managers, created_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			 RETURNING `+resourceCols,
			resourceID, t.Slug, calendarID, string(nr.Kind), strings.TrimSpace(nr.Name),
			nr.Capacity, nr.Location, domain.NormalizeEmail(nr.Email), nr.AutoAccept, nr.Managers, nr.CreatedBy)
		var e error
		r, e = scanResource(row, t.Slug)
		return e
	})
	return r, err
}

// GetResource devuelve un recurso vivo por ID.
func (s *Store) GetResource(ctx context.Context, id uuid.UUID) (domain.Resource, error) {
	t := tenancy.MustFrom(ctx)
	var r domain.Resource
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+resourceCols+` FROM calendar_resources
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, t.Slug, id)
		var e error
		r, e = scanResource(row, t.Slug)
		return e
	})
	if isNoRows(err) {
		return domain.Resource{}, domain.ErrNotFound
	}
	return r, err
}

// ResourceAvailability es un recurso más sus periodos OCUPADOS en la ventana
// consultada (free/busy de su calendario de reservas). Busy vacío => libre.
type ResourceAvailability struct {
	Resource domain.Resource         `json:"resource"`
	Busy     []domain.FreeBusyPeriod `json:"busy"`
}

// ListResources lista los recursos activos del tenant. Si withAvailability, además
// calcula los periodos ocupados de cada recurso en [from, to) reutilizando la
// expansión de recurrencias y el motor free/busy del dominio. Los eventos de TODOS
// los calendarios de recursos se leen en UNA sola consulta (sin N+1) y se agrupan
// por recurso en memoria.
func (s *Store) ListResources(ctx context.Context, from, to time.Time, withAvailability bool) ([]ResourceAvailability, error) {
	t := tenancy.MustFrom(ctx)
	var out []ResourceAvailability
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+resourceCols+` FROM calendar_resources
			 WHERE tenant=$1 AND active AND deleted_at IS NULL
			 ORDER BY lower(name), id`, t.Slug)
		if err != nil {
			return fmt.Errorf("store: listar recursos: %w", err)
		}
		defer rows.Close()
		idx := map[uuid.UUID]int{}
		for rows.Next() {
			r, err := scanResource(rows, t.Slug)
			if err != nil {
				return err
			}
			idx[r.ID] = len(out)
			out = append(out, ResourceAvailability{Resource: r, Busy: []domain.FreeBusyPeriod{}})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if !withAvailability || len(out) == 0 || !to.After(from) {
			return nil
		}
		return attachResourceBusy(ctx, tx, t.Slug, out, idx, from.UTC(), to.UTC())
	})
	return out, err
}

// attachResourceBusy carga en UNA consulta los eventos en rango de TODOS los
// calendarios de recursos del lote y, por recurso, los expande y fusiona en periodos
// ocupados (free/busy). Evita el N+1 (una consulta por recurso).
func attachResourceBusy(ctx context.Context, tx pgx.Tx, tenant string, list []ResourceAvailability, idx map[uuid.UUID]int, from, to time.Time) error {
	rows, err := tx.Query(ctx,
		`SELECT r.id, `+eventQual("e")+`
		   FROM calendar_resources r
		   JOIN events e ON e.calendar_id = r.calendar_id AND e.tenant = r.tenant AND e.deleted_at IS NULL
		  WHERE r.tenant=$1 AND r.active AND r.deleted_at IS NULL
		    AND e.dtstart < $3
		    AND (e.recurrence_end IS NULL OR e.recurrence_end > $2)
		    AND (
		         (e.rrule = '' AND cardinality(e.rdate) = 0 AND e.dtend > $2)
		      OR (e.rrule <> '' OR cardinality(e.rdate) > 0)
		    )
		  ORDER BY r.id, e.dtstart`, tenant, from, to)
	if err != nil {
		return fmt.Errorf("store: disponibilidad de recursos: %w", err)
	}
	defer rows.Close()
	byRes := map[uuid.UUID][]domain.Event{}
	for rows.Next() {
		var resID uuid.UUID
		ev, err := scanResourceEvent(rows, tenant, &resID)
		if err != nil {
			return err
		}
		byRes[resID] = append(byRes[resID], ev)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for resID, evs := range byRes {
		i, ok := idx[resID]
		if !ok {
			continue
		}
		busy := domain.FreeBusy(domain.ExpandEvents(evs, from, to), from, to)
		if busy != nil {
			list[i].Busy = busy
		}
	}
	return nil
}

// scanResourceEvent escanea una fila «resource_id + columnas de evento» (en el orden
// de eventCols) en un domain.Event, dejando el id del recurso en out.
func scanResourceEvent(row pgx.Row, tenant string, resID *uuid.UUID) (domain.Event, error) {
	var e domain.Event
	var status, class, transp string
	if err := row.Scan(resID, &e.ID, &e.CalendarID, &e.ObjectID, &e.UID, &e.Summary, &e.Description,
		&e.Location, &e.Organizer, &e.DTStart, &e.DTEnd, &e.TZID, &e.AllDay,
		&status, &class, &transp, &e.Color, &e.ConferenceURL, &e.RRule, &e.RDate, &e.ExDate, &e.RecurrenceEnd,
		&e.RecurrenceID, &e.SequenceNo, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return domain.Event{}, err
	}
	e.Tenant = tenant
	e.Status = domain.EventStatus(status)
	e.Classification = domain.Classification(class)
	e.Transparency = domain.Transparency(transp)
	return e, nil
}

// ResourcePatch son los campos editables de un recurso (nil = no cambia). Managers,
// si no es nil, REEMPLAZA la lista completa. El tipo (kind) es inmutable.
type ResourcePatch struct {
	Name       *string
	Capacity   *int
	Location   *string
	Email      *string
	AutoAccept *bool
	Active     *bool
	Managers   *[]string
}

// UpdateResource aplica un patch a un recurso. Solo el gestor (managers/created_by)
// o un admin del tenant (isAdmin) — si no, ErrNotManager (403).
func (s *Store) UpdateResource(ctx context.Context, id uuid.UUID, sub string, isAdmin bool, p ResourcePatch) (domain.Resource, error) {
	t := tenancy.MustFrom(ctx)
	var r domain.Resource
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		cur, err := scanResource(tx.QueryRow(ctx,
			`SELECT `+resourceCols+` FROM calendar_resources
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, t.Slug, id), t.Slug)
		if err != nil {
			return err
		}
		if !isAdmin && !cur.IsManager(sub) {
			return domain.ErrNotManager
		}
		var email *string
		if p.Email != nil {
			norm := domain.NormalizeEmail(*p.Email)
			email = &norm
		}
		row := tx.QueryRow(ctx,
			`UPDATE calendar_resources SET
				name        = COALESCE($3, name),
				capacity    = COALESCE($4, capacity),
				location    = COALESCE($5, location),
				email       = COALESCE($6, email),
				auto_accept = COALESCE($7, auto_accept),
				active      = COALESCE($8, active),
				managers    = COALESCE($9, managers),
				updated_at  = now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL
			 RETURNING `+resourceCols,
			t.Slug, id, p.Name, p.Capacity, p.Location, email, p.AutoAccept, p.Active, p.Managers)
		r, err = scanResource(row, t.Slug)
		return err
	})
	if isNoRows(err) {
		return domain.Resource{}, domain.ErrNotFound
	}
	return r, err
}

// DeleteResource desactiva (soft) un recurso y su calendario de reservas. Solo el
// gestor o un admin. El calendario se marca borrado (sus reservas dejan de contar).
func (s *Store) DeleteResource(ctx context.Context, id uuid.UUID, sub string, isAdmin bool) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		cur, err := scanResource(tx.QueryRow(ctx,
			`SELECT `+resourceCols+` FROM calendar_resources
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, t.Slug, id), t.Slug)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if !isAdmin && !cur.IsManager(sub) {
			return domain.ErrNotManager
		}
		if _, err := tx.Exec(ctx,
			`UPDATE calendar_resources SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND id=$2`, t.Slug, id); err != nil {
			return fmt.Errorf("store: borrar recurso: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE calendars SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND id=$2`, t.Slug, cur.CalendarID); err != nil {
			return fmt.Errorf("store: borrar calendario de recurso: %w", err)
		}
		return nil
	})
}

// ResourceBooking son los datos de una reserva a comprobar/persistir en el
// calendario del recurso (la programación del evento origen). RecurrenceEnd la
// deriva el servicio de la RRULE (como en un evento normal).
type ResourceBooking struct {
	SourceUID     string
	Summary       string
	Organizer     string
	DTStart       time.Time
	DTEnd         time.Time
	TZID          string
	AllDay        bool
	RRule         string
	RDate         []time.Time
	ExDate        []time.Time
	RecurrenceEnd *time.Time
}

// BookingResult es el resultado de una reserva: el recurso, el estado resultante
// (ACCEPTED / DECLINED / TENTATIVE=pendiente de aprobación manual) y, si se rechazó,
// el periodo ocupado que causó el conflicto.
type BookingResult struct {
	Resource domain.Resource
	Status   domain.ParticipationStatus
	Conflict *domain.FreeBusyPeriod
}

// bookingUID es el UID de la reserva en el calendario del recurso: deriva del UID del
// evento origen y del recurso, de modo que hay UNA reserva por (evento, recurso) y
// reprogramar el evento reutiliza (no duplica) su reserva.
func bookingUID(resourceID uuid.UUID, sourceUID string) string {
	return sourceUID + "@resource-" + resourceID.String()
}

// BookResource reserva ATÓMICAMENTE el recurso para la programación dada. Toma un
// bloqueo de fila del recurso (FOR UPDATE) y, con él, comprueba el solape (respetando
// recurrencia + TZ) y, si está libre, inserta la reserva en el calendario del recurso
// — todo en una transacción. Devuelve ACCEPTED (auto-accept libre), TENTATIVE
// (libre pero requiere aprobación por auto_accept=false) o DECLINED (ocupado). Es
// idempotente por (evento, recurso): reprogramar sustituye la reserva anterior.
func (s *Store) BookResource(ctx context.Context, resourceID uuid.UUID, b ResourceBooking) (BookingResult, error) {
	t := tenancy.MustFrom(ctx)
	var res BookingResult
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Bloqueo de fila del recurso: serializa TODAS las reservas de este recurso.
		resource, err := scanResource(tx.QueryRow(ctx,
			`SELECT `+resourceCols+` FROM calendar_resources
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL
			 FOR UPDATE`, t.Slug, resourceID), t.Slug)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if !resource.Active {
			return domain.ErrResourceInactive
		}
		res.Resource = resource

		uid := bookingUID(resourceID, b.SourceUID)
		from := b.DTStart.UTC()
		to := b.DTEnd.UTC()
		if b.RRule != "" || len(b.RDate) > 0 {
			if b.RecurrenceEnd != nil {
				to = b.RecurrenceEnd.UTC()
			} else {
				to = from.Add(maxConflictWindow)
			}
		}
		if !to.After(from) {
			to = from.Add(time.Minute)
		}

		existing, err := listEventsInRangeTx(ctx, tx, t.Slug, resource.CalendarID, from, to)
		if err != nil {
			return err
		}
		candidate := domain.Event{
			UID: uid, DTStart: from, DTEnd: b.DTEnd.UTC(), TZID: orTZ(b.TZID), AllDay: b.AllDay,
			Status: domain.StatusConfirmed, Transparency: domain.TranspOpaque,
			RRule: b.RRule, RDate: b.RDate, ExDate: b.ExDate, RecurrenceEnd: b.RecurrenceEnd,
		}
		if period, conflict := domain.ResourceConflict(existing, candidate, from, to); conflict {
			res.Status = domain.PartDeclined
			p := period
			res.Conflict = &p
			return nil // ocupado: NO se inserta la reserva
		}

		status := domain.StatusConfirmed
		res.Status = domain.PartAccepted
		if !resource.AutoAccept {
			status = domain.StatusTentative // hold pendiente de aprobación manual
			res.Status = domain.PartTentative
		}

		// Reprograma: retira una reserva previa del mismo (evento, recurso) antes de
		// insertar la nueva (el índice único es sobre las filas vivas).
		if err := softDeleteObjectByUID(ctx, tx, t.Slug, resource.CalendarID, uid); err != nil {
			return err
		}
		if err := insertBookingTx(ctx, tx, t.Slug, resource.CalendarID, uid, status, b); err != nil {
			return err
		}
		return bumpCalendar(ctx, tx, t.Slug, resource.CalendarID)
	})
	if err != nil {
		return BookingResult{}, err
	}
	return res, nil
}

// ReleaseResourceBooking libera la reserva del (evento, recurso): borra la reserva
// del calendario del recurso. Autorizado para quien la reservó (organizer == email),
// un gestor del recurso o un admin del tenant; si no, ErrNotManager (403).
func (s *Store) ReleaseResourceBooking(ctx context.Context, resourceID uuid.UUID, sourceUID, callerSub, callerEmail string, isAdmin bool) error {
	t := tenancy.MustFrom(ctx)
	callerEmail = domain.NormalizeEmail(callerEmail)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		resource, err := scanResource(tx.QueryRow(ctx,
			`SELECT `+resourceCols+` FROM calendar_resources
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, t.Slug, resourceID), t.Slug)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		uid := bookingUID(resourceID, sourceUID)
		var organizer string
		err = tx.QueryRow(ctx,
			`SELECT organizer FROM events
			 WHERE tenant=$1 AND calendar_id=$2 AND uid=$3 AND recurrence_id IS NULL AND deleted_at IS NULL`,
			t.Slug, resource.CalendarID, uid).Scan(&organizer)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if !isAdmin && !resource.IsManager(callerSub) &&
			(callerEmail == "" || domain.NormalizeEmail(organizer) != callerEmail) {
			return domain.ErrNotManager
		}
		if err := softDeleteObjectByUID(ctx, tx, t.Slug, resource.CalendarID, uid); err != nil {
			return err
		}
		return bumpCalendar(ctx, tx, t.Slug, resource.CalendarID)
	})
}

// listEventsInRangeTx lee, dentro de una tx, los eventos vivos de un calendario con
// posibles instancias en [from, to). Misma lógica que ListEventsInRange (la expansión
// la hace el dominio); aquí sin abrir otra transacción (para correr bajo el lock).
func listEventsInRangeTx(ctx context.Context, tx pgx.Tx, tenant string, calendarID uuid.UUID, from, to time.Time) ([]domain.Event, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+eventCols+` FROM events
		 WHERE tenant=$1 AND calendar_id=$2 AND deleted_at IS NULL
		   AND dtstart < $4
		   AND (
		        (rrule = '' AND cardinality(rdate) = 0 AND dtend > $3)
		     OR (rrule <> '' OR cardinality(rdate) > 0)
		   )
		   AND (recurrence_end IS NULL OR recurrence_end > $3)
		 ORDER BY dtstart, id`,
		tenant, calendarID, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("store: eventos del recurso por rango: %w", err)
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		ev, err := scanEvent(rows, tenant)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// insertBookingTx inserta el objeto CalDAV (iCalendar mínimo) y el evento maestro de
// una reserva en el calendario del recurso, con el estado dado (CONFIRMED al aceptar,
// TENTATIVE si queda pendiente de aprobación).
func insertBookingTx(ctx context.Context, tx pgx.Tx, tenant string, calendarID uuid.UUID, uid string, status domain.EventStatus, b ResourceBooking) error {
	ics := minimalBookingICS(uid, b.Summary, b.DTStart, b.DTEnd)
	var objID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO calendar_objects (tenant, calendar_id, uid, resource_name, component, data, size_bytes, sequence_no)
		 VALUES ($1, $2, $3, $4, 'VEVENT', $5, $6, 0)
		 RETURNING id`,
		tenant, calendarID, uid, uid+".ics", ics, len(ics)).Scan(&objID); err != nil {
		if uniqueViolation(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("store: crear objeto de reserva: %w", err)
	}
	in := domain.EventInput{
		CalendarID: calendarID, UID: uid, ResourceName: uid + ".ics", ICalData: ics,
		Summary: b.Summary, Organizer: b.Organizer,
		DTStart: b.DTStart.UTC(), DTEnd: b.DTEnd.UTC(), TZID: orTZ(b.TZID), AllDay: b.AllDay,
		Status: status, Classification: domain.ClassPublic, Transparency: domain.TranspOpaque,
		RRule: b.RRule, RDate: b.RDate, ExDate: b.ExDate, RecurrenceEnd: b.RecurrenceEnd,
	}
	_, err := insertEventRow(ctx, tx, tenant, objID, in)
	return err
}

// softDeleteObjectByUID marca borrados (soft) el objeto CalDAV y sus eventos por UID
// en un calendario (para reprogramar/liberar una reserva). No-op si no existe.
func softDeleteObjectByUID(ctx context.Context, tx pgx.Tx, tenant string, calendarID uuid.UUID, uid string) error {
	if _, err := tx.Exec(ctx,
		`UPDATE events SET deleted_at=now(), updated_at=now()
		 WHERE tenant=$1 AND deleted_at IS NULL AND object_id IN (
		     SELECT id FROM calendar_objects
		     WHERE tenant=$1 AND calendar_id=$2 AND uid=$3 AND deleted_at IS NULL
		 )`, tenant, calendarID, uid); err != nil {
		return fmt.Errorf("store: retirar eventos de reserva: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE calendar_objects SET deleted_at=now(), updated_at=now()
		 WHERE tenant=$1 AND calendar_id=$2 AND uid=$3 AND deleted_at IS NULL`,
		tenant, calendarID, uid); err != nil {
		return fmt.Errorf("store: retirar objeto de reserva: %w", err)
	}
	return nil
}

// minimalBookingICS sintetiza un VCALENDAR/VEVENT mínimo y válido para el objeto
// CalDAV de una reserva. Los detalles consultables (rango, recurrencia, estado) viven
// en las columnas de `events`; este blob es el recurso serializado de respaldo.
func minimalBookingICS(uid, summary string, start, end time.Time) string {
	const layout = "20060102T150405Z"
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Calendar Resource//EN\r\n")
	b.WriteString("BEGIN:VEVENT\r\nUID:")
	b.WriteString(uid)
	b.WriteString("\r\nDTSTAMP:")
	b.WriteString(time.Now().UTC().Format(layout))
	b.WriteString("\r\nDTSTART:")
	b.WriteString(start.UTC().Format(layout))
	b.WriteString("\r\nDTEND:")
	b.WriteString(end.UTC().Format(layout))
	b.WriteString("\r\nSUMMARY:")
	b.WriteString(icalEscapeText(summary))
	b.WriteString("\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	return b.String()
}

// icalEscapeText escapa el texto para una propiedad iCalendar (RFC 5545 3.3.11):
// backslash, coma, punto y coma y saltos de línea.
func icalEscapeText(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`, "\r", "")
	return r.Replace(s)
}

func orTZ(tz string) string {
	if tz == "" {
		return "UTC"
	}
	return tz
}
