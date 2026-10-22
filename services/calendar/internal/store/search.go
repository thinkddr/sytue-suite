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

// EventSearchCursor es el punto de continuación keyset de una búsqueda de eventos:
// la última (dtstart, id) devuelta. nil = primera página. Orden ASCENDENTE y
// estable por (dtstart, id), sin OFFSET (CONVENTIONS §0.8).
type EventSearchCursor struct {
	DTStart time.Time
	ID      uuid.UUID
}

// EventSearchQuery son los criterios de una búsqueda de eventos. Campos punteros
// vacíos no filtran.
type EventSearchQuery struct {
	// OwnerSub restringe a los calendarios de ESTE usuario (aislamiento por dueño,
	// además del tenant por RLS). Obligatorio.
	OwnerSub string
	// Text es el término de búsqueda (subcadena, case-insensitive) sobre
	// título/descripción/lugar/organizador y email/nombre de asistentes.
	Text string
	// CalendarID, si no es Nil, restringe a un calendario del usuario.
	CalendarID uuid.UUID
	// From/To acotan por rango temporal (opcionales). From incluye las series cuya
	// recurrencia alcanza la ventana (no solo su instante maestro).
	From *time.Time
	To   *time.Time
	// After es el cursor keyset (nil = primera página).
	After *EventSearchCursor
	// Limit acota el tamaño de la página.
	Limit int
}

// minSearchLen es la longitud mínima del término: por debajo no se busca (evita
// volcar el calendario con un término trivial). Para 1-2 caracteres el GIN de
// trigramas no aplica y caería a scan; el mínimo lo evita.
const minSearchLen = 2

// SearchEvents busca los eventos MAESTROS (una fila por serie/evento; sin expandir
// ocurrencias) de un usuario cuyo texto casa el término, con rango de fechas y
// filtro por calendario opcionales, paginados por keyset (dtstart, id) ascendente.
//
// Aislamiento: corre bajo RLS (tenant del contexto, InTenantTx) y además JOINea
// calendars por owner_sub, de modo que solo devuelve eventos de los calendarios
// del propio usuario — nunca de otros dueños del mismo tenant. El término va
// SIEMPRE parametrizado (sin SQL dinámico ni interpolación): no hay inyección.
// Una sola consulta indexada (GIN trgm + keyset), sin N+1.
func (s *Store) SearchEvents(ctx context.Context, q EventSearchQuery) ([]domain.Event, error) {
	t := tenancy.MustFrom(ctx)
	term := strings.TrimSpace(q.Text)
	if len(term) < minSearchLen || q.OwnerSub == "" {
		return nil, nil
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// calendarID/from/to/cursor como NULL cuando no se filtran: la consulta es la
	// misma para todos los casos (el planificador poda los IS NULL).
	var calID any
	if q.CalendarID != uuid.Nil {
		calID = q.CalendarID
	}
	var from, to any
	if q.From != nil {
		from = q.From.UTC()
	}
	if q.To != nil {
		to = q.To.UTC()
	}
	var curTS, curID any
	if q.After != nil {
		curTS = q.After.DTStart.UTC()
		curID = q.After.ID
	}

	var out []domain.Event
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// ponytail: ILIKE sin escapar %/_ (metacaracteres LIKE), igual que
		// contacts/mail/drive; es relevancia de búsqueda, no inyección (el término
		// va parametrizado en $3). El rango filtra por el instante maestro y, para
		// series, por su recurrence_end (misma lógica de solape que ListEventsInRange);
		// se devuelve el maestro (marcado recurrente por su rrule/rdate), no cada
		// ocurrencia. Próxima-ocurrencia exacta por serie = upgrade path.
		rows, err := tx.Query(ctx,
			`SELECT `+eventQual("e")+` FROM events e
			 JOIN calendars c ON c.id = e.calendar_id AND c.tenant = e.tenant
			 WHERE e.tenant = $1 AND c.owner_sub = $2
			   AND e.deleted_at IS NULL AND c.deleted_at IS NULL
			   AND e.recurrence_id IS NULL
			   AND ($4::uuid IS NULL OR e.calendar_id = $4)
			   AND ($5::timestamptz IS NULL OR (
			         (e.rrule = '' AND cardinality(e.rdate) = 0 AND e.dtend > $5)
			      OR ((e.rrule <> '' OR cardinality(e.rdate) > 0)
			          AND (e.recurrence_end IS NULL OR e.recurrence_end > $5))
			   ))
			   AND ($6::timestamptz IS NULL OR e.dtstart < $6)
			   AND (
			         e.search_text LIKE '%' || plegar($3) || '%'
			      OR EXISTS (
			           SELECT 1 FROM attendees a
			           WHERE a.tenant = e.tenant AND a.event_id = e.id
			             AND plegar(a.email || ' ' || a.common_name) LIKE '%' || plegar($3) || '%'
			         )
			   )
			   AND ($7::timestamptz IS NULL OR (e.dtstart, e.id) > ($7, $8))
			 ORDER BY e.dtstart, e.id
			 LIMIT $9`,
			t.Slug, q.OwnerSub, term, calID, from, to, curTS, curID, limit)
		if err != nil {
			return fmt.Errorf("store: buscar eventos: %w", err)
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
