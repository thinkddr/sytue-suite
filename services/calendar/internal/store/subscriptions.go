// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// Suscripciones ICS externas. Una suscripción es un calendario de SOLO LECTURA
// (calendars.read_only) más una fila calendar_subscriptions (1:1) con el estado
// de sincronización. Sus VEVENT se cachean como objetos/eventos normales, así el
// render/CalDAV/RRULE/TZ se reutilizan; el refresco los reemplaza en bloque.

// NewSubscription son los datos de alta de una suscripción ICS.
type NewSubscription struct {
	OwnerSub        string
	URL             string
	DisplayName     string
	Color           string
	RefreshInterval time.Duration
}

// CreateSubscription crea el calendario de solo lectura y su fila de
// suscripción en UNA transacción. name es un slug único derivado del display
// name; nace con next_refresh_at=now() para que el worker lo obtenga en el
// primer ciclo. Devuelve el calendario ya con su Subscription adjunta.
func (s *Store) CreateSubscription(ctx context.Context, name string, ns NewSubscription) (domain.Calendar, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidName(name) {
		return domain.Calendar{}, domain.ErrInvalidName
	}
	if !domain.ValidSubscriptionURL(ns.URL) {
		return domain.Calendar{}, domain.ErrInvalidURL
	}
	color := ns.Color
	if color == "" {
		color = "#6B3E9E"
	}
	interval := domain.ClampRefreshInterval(ns.RefreshInterval)

	var c domain.Calendar
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`INSERT INTO calendars (tenant, owner_sub, name, display_name, color, read_only)
			 VALUES ($1, $2, $3, $4, $5, true)
			 RETURNING `+calendarCols,
			t.Slug, ns.OwnerSub, name, ns.DisplayName, color)
		var e error
		c, e = scanCalendar(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		if e != nil {
			return e
		}
		if _, e := tx.Exec(ctx,
			`INSERT INTO calendar_subscriptions (tenant, calendar_id, url, refresh_interval_seconds)
			 VALUES ($1, $2, $3, $4)`,
			t.Slug, c.ID, ns.URL, int(interval.Seconds())); e != nil {
			return fmt.Errorf("store: crear suscripción: %w", e)
		}
		return attachSubscriptions(ctx, tx, t.Slug, []domain.Calendar{c})
	})
	if err != nil {
		return domain.Calendar{}, err
	}
	// attachSubscriptions muta la copia del slice; recárgala para el retorno.
	return s.GetCalendar(ctx, c.ID)
}

// attachSubscriptions carga en bloque (sin N+1) el estado de suscripción de los
// calendarios dados y lo adjunta (Subscription != nil). Debe llamarse dentro de
// la tx que ya leyó los calendarios.
func attachSubscriptions(ctx context.Context, tx pgx.Tx, tenant string, cals []domain.Calendar) error {
	if len(cals) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(cals))
	for _, c := range cals {
		if c.ReadOnly {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return nil // ninguno es de solo lectura => ninguno tiene suscripción
	}
	rows, err := tx.Query(ctx,
		`SELECT calendar_id, url, refresh_interval_seconds, etag, last_modified,
		        last_fetched_at, next_refresh_at, last_status, last_error
		   FROM calendar_subscriptions
		  WHERE tenant=$1 AND calendar_id = ANY($2)`, tenant, ids)
	if err != nil {
		return fmt.Errorf("store: leer suscripciones: %w", err)
	}
	defer rows.Close()
	byCal := make(map[uuid.UUID]*domain.Subscription, len(ids))
	for rows.Next() {
		var calID uuid.UUID
		var sub domain.Subscription
		if err := rows.Scan(&calID, &sub.URL, &sub.RefreshInterval, &sub.ETag, &sub.LastModified,
			&sub.LastFetchedAt, &sub.NextRefreshAt, &sub.LastStatus, &sub.LastError); err != nil {
			return err
		}
		s := sub
		byCal[calID] = &s
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range cals {
		if sub, ok := byCal[cals[i].ID]; ok {
			cals[i].Subscription = sub
		}
	}
	return nil
}

// SubJob es lo que el worker/refresh-now necesita para refrescar una suscripción:
// identidad + URL + validadores condicionales + cadencia. Sin datos de usuario.
type SubJob struct {
	ID              uuid.UUID
	Tenant          string
	CalendarID      uuid.UUID
	URL             string
	ETag            string
	LastModified    string
	RefreshInterval time.Duration
}

const subJobCols = `id, tenant, calendar_id, url, etag, last_modified, refresh_interval_seconds`

func scanSubJob(row pgx.Row) (SubJob, error) {
	var j SubJob
	var secs int
	if err := row.Scan(&j.ID, &j.Tenant, &j.CalendarID, &j.URL, &j.ETag, &j.LastModified, &secs); err != nil {
		return SubJob{}, err
	}
	j.RefreshInterval = time.Duration(secs) * time.Second
	return j, nil
}

// SubscriptionForCalendar resuelve la suscripción de un calendario del tenant
// (para «refrescar ahora»). ErrNotFound si el calendario no es una suscripción.
func (s *Store) SubscriptionForCalendar(ctx context.Context, calendarID uuid.UUID) (SubJob, error) {
	t := tenancy.MustFrom(ctx)
	var j SubJob
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+subJobCols+` FROM calendar_subscriptions WHERE tenant=$1 AND calendar_id=$2`,
			t.Slug, calendarID)
		var e error
		j, e = scanSubJob(row)
		return e
	})
	if isNoRows(err) {
		return SubJob{}, domain.ErrNotFound
	}
	return j, err
}

// ClaimDueSubscriptions reclama hasta `limit` suscripciones cuyo refresco venció,
// ARRENDÁNDOLAS: empuja su next_refresh_at un intervalo hacia delante en el mismo
// UPDATE, de modo que otra réplica del worker no las tome a la vez. Cross-tenant
// (app.maintenance, política PERMISSIVE), FOR UPDATE SKIP LOCKED. El resultado
// real del fetch afina luego next_refresh_at (MarkSubscriptionResult).
func (s *Store) ClaimDueSubscriptions(ctx context.Context, limit int) ([]SubJob, error) {
	if limit <= 0 {
		limit = 20
	}
	var jobs []SubJob
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`UPDATE calendar_subscriptions s
			    SET next_refresh_at = now() + make_interval(secs => s.refresh_interval_seconds),
			        updated_at = now()
			  WHERE s.id IN (
			        SELECT id FROM calendar_subscriptions
			         WHERE next_refresh_at <= now()
			         ORDER BY next_refresh_at
			         FOR UPDATE SKIP LOCKED
			         LIMIT $1)
			  RETURNING `+subJobColsQual(`s`), limit)
		if err != nil {
			return fmt.Errorf("store: reclamar suscripciones: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			j, err := scanSubJob(rows)
			if err != nil {
				return err
			}
			jobs = append(jobs, j)
		}
		return rows.Err()
	})
	return jobs, err
}

// SubscriptionResult es el desenlace de un refresco, que MarkSubscriptionResult
// persiste (estado + validadores + próximo refresco).
type SubscriptionResult struct {
	Status       string // "ok" | "not_modified" | "error"
	ETag         string
	LastModified string
	LastError    string
	NextRefresh  time.Time
}

// MarkSubscriptionResult persiste el desenlace de un refresco. Tenant-scoped
// (el llamante fija el tenant del job en el contexto). En "not_modified" conserva
// los validadores previos (no se pasan vacíos); en "ok" los actualiza.
func (s *Store) MarkSubscriptionResult(ctx context.Context, job SubJob, res SubscriptionResult) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE calendar_subscriptions
			    SET etag = $3, last_modified = $4, last_status = $5, last_error = $6,
			        last_fetched_at = now(), next_refresh_at = $7, updated_at = now()
			  WHERE tenant=$1 AND id=$2`,
			t.Slug, job.ID, res.ETag, res.LastModified, res.Status, res.LastError, res.NextRefresh.UTC())
		if err != nil {
			return fmt.Errorf("store: marcar resultado de suscripción: %w", err)
		}
		return nil
	})
}

// SubscriptionObject es un recurso decodificado del feed (maestro + overrides de
// recurrencia del mismo UID) listo para cachear. El iCalendar verbatim va en
// Master.ICalData.
type SubscriptionObject struct {
	Master    domain.EventInput
	Overrides []domain.EventInput
}

// SyncSubscriptionObjects REEMPLAZA en bloque los objetos/eventos cacheados de un
// calendario de suscripción por los del feed recién obtenido. Hard-delete de los
// objetos previos (cascada a eventos/asistentes) + reinserción, y avanza el ctag.
// Es el ÚNICO camino de escritura de un calendario read_only: no pasa por la
// puerta de solo-lectura (assertWritable), que solo protege las escrituras de
// USUARIO. Tenant-scoped (el llamante fija el tenant del job en el contexto).
func (s *Store) SyncSubscriptionObjects(ctx context.Context, calendarID uuid.UUID, objs []SubscriptionObject) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// El calendario debe existir, estar vivo y ser de solo lectura (una
		// suscripción): no reescribimos el contenido de un calendario editable.
		var readOnly bool
		if err := tx.QueryRow(ctx,
			`SELECT read_only FROM calendars WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, calendarID).Scan(&readOnly); err != nil {
			if isNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if !readOnly {
			return domain.ErrReadOnly
		}
		// Reemplazo total: borra los objetos previos (cascada a events/attendees).
		if _, err := tx.Exec(ctx,
			`DELETE FROM calendar_objects WHERE tenant=$1 AND calendar_id=$2`, t.Slug, calendarID); err != nil {
			return fmt.Errorf("store: limpiar objetos de suscripción: %w", err)
		}
		for _, o := range objs {
			objID, err := insertSubObject(ctx, tx, t.Slug, calendarID, o.Master)
			if err != nil {
				return err
			}
			if _, err := insertEventRow(ctx, tx, t.Slug, objID, withDefaults(o.Master)); err != nil {
				return err
			}
			for _, ov := range o.Overrides {
				if _, err := insertEventRow(ctx, tx, t.Slug, objID, withDefaults(ov)); err != nil {
					return err
				}
			}
		}
		return bumpCalendar(ctx, tx, t.Slug, calendarID)
	})
}

// insertSubObject inserta el recurso CalDAV (calendar_objects) de un objeto del
// feed y devuelve su id (para colgar los eventos). El UID/resource vienen del
// feed; el iCalendar se guarda verbatim para servirlo por CalDAV.
func insertSubObject(ctx context.Context, tx pgx.Tx, tenant string, calendarID uuid.UUID, m domain.EventInput) (uuid.UUID, error) {
	resource := m.ResourceName
	if resource == "" {
		resource = m.UID + ".ics"
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx,
		`INSERT INTO calendar_objects (tenant, calendar_id, uid, resource_name, component, data, size_bytes, sequence_no)
		 VALUES ($1, $2, $3, $4, 'VEVENT', $5, $6, $7)
		 RETURNING id`,
		tenant, calendarID, m.UID, resource, m.ICalData, len(m.ICalData), m.SequenceNo).Scan(&id)
	if uniqueViolation(err) {
		return uuid.Nil, domain.ErrConflict
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("store: insertar objeto de suscripción: %w", err)
	}
	return id, nil
}

// subJobColsQual devuelve subJobCols con cada columna cualificada por alias (para
// el RETURNING de un UPDATE con alias).
func subJobColsQual(alias string) string {
	return alias + ".id, " + alias + ".tenant, " + alias + ".calendar_id, " + alias + ".url, " +
		alias + ".etag, " + alias + ".last_modified, " + alias + ".refresh_interval_seconds"
}
