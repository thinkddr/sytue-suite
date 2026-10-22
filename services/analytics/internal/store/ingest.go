// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/analytics/internal/domain"
)

// eventColumns es el orden de columnas del INSERT del firehose.
const eventColumns = `tenant, site_id, ts, visitor, name, path, referrer_host, target,
	country, browser, os, device, utm_source, utm_medium, utm_campaign, utm_content, utm_term, ref`

// eventFields son los parámetros por fila (debe coincidir con eventColumns).
const eventFields = 18

// Writer es el escritor por lotes del colector.
//
// Va con INSERT multi-fila y no con COPY … FROM por una razón que no es de
// gusto: bajo FORCE ROW LEVEL SECURITY PostgreSQL RECHAZA COPY … FROM. El
// diseño obvio de firehose no está disponible, así que se agrupa (~1 s o N
// eventos, lo que llegue antes) y se escribe con una sola sentencia.
//
// La cola es de tamaño fijo y el encolado NO BLOQUEA: si se llena, se descartan
// eventos y se cuenta cuántos. Perder muestras es infinitamente preferible a que
// el colector —público y sin autenticar— retenga peticiones de visitantes de
// todos los clientes de la plataforma.
type Writer struct {
	st       *Store
	ch       chan domain.Event
	size     int
	interval time.Duration
	dropped  atomic.Int64
	written  atomic.Int64
}

// NewWriter crea el escritor. queue es la cola en memoria, size el tamaño de
// lote e interval el intervalo máximo entre escrituras.
func NewWriter(st *Store, queue, size int, interval time.Duration) *Writer {
	return &Writer{st: st, ch: make(chan domain.Event, queue), size: size, interval: interval}
}

// Enqueue encola un evento. Devuelve false si la cola está llena (evento
// descartado): el llamante NO debe esperar ni reintentar.
func (w *Writer) Enqueue(e domain.Event) bool {
	select {
	case w.ch <- e:
		return true
	default:
		w.dropped.Add(1)
		return false
	}
}

// Stats devuelve los contadores acumulados (escritos, descartados).
func (w *Writer) Stats() (written, dropped int64) {
	return w.written.Load(), w.dropped.Load()
}

// Run bombea la cola hasta que se cancela el contexto, momento en el que drena
// lo pendiente con un plazo propio.
//
//nolint:contextcheck // el drenaje final usa un contexto nuevo A PROPÓSITO: el padre ya está cancelado.
func (w *Writer) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	batch := make([]domain.Event, 0, w.size)

	flush := func(c context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := w.st.insertEvents(c, batch); err != nil {
			log.From(ctx).Error("lote de eventos perdido", "error", err, "eventos", len(batch))
		} else {
			w.written.Add(int64(len(batch)))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			for {
				select {
				case e := <-w.ch:
					batch = append(batch, e)
					if len(batch) >= w.size {
						flush(drainCtx)
					}
					continue
				default:
				}
				break
			}
			flush(drainCtx)
			cancel()
			return nil
		case e := <-w.ch:
			batch = append(batch, e)
			if len(batch) >= w.size {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}

// insertEvents escribe el lote en una sola sentencia bajo app.ingest='on'.
func (s *Store) insertEvents(ctx context.Context, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	sql, args := buildInsert(events)
	err := s.inIngestTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
	if err == nil {
		return nil
	}
	// «no partition of relation … found for row» (23514): el cron aún no ha
	// creado la del día. Se crean y se reintenta UNA vez; sin esto, un despliegue
	// a caballo de la medianoche perdería el lote entero.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		if perr := s.EnsurePartitions(ctx, 7); perr != nil {
			return fmt.Errorf("analytics: insertar eventos: %w (crear particiones: %w)", err, perr)
		}
		return s.inIngestTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)
			return err
		})
	}
	return fmt.Errorf("analytics: insertar eventos: %w", err)
}

// buildInsert compone el INSERT multi-fila con placeholders (nunca interpolando
// valores: todo lo que hay aquí viene del navegador).
func buildInsert(events []domain.Event) (string, []any) {
	var b strings.Builder
	b.Grow(128 + len(events)*eventFields*5)
	// En una sola cadena, para que el guardián del SQL (sqlorg) vea que `tenant` va
	// en la lista de columnas.
	b.WriteString("INSERT INTO analytics_events (" + eventColumns + ") VALUES ")

	args := make([]any, 0, len(events)*eventFields)
	for i, e := range events {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('(')
		for j := range eventFields {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(i*eventFields + j + 1))
		}
		b.WriteByte(')')
		args = append(args,
			e.Tenant, e.SiteID, e.TS, e.Visitor, e.Name, e.Path,
			nullable(e.Referrer), nullable(e.Target), e.Country, e.Browser, e.OS, e.Device,
			nullable(e.UTM[0]), nullable(e.UTM[1]), nullable(e.UTM[2]),
			nullable(e.UTM[3]), nullable(e.UTM[4]), nullable(e.UTM[5]))
	}
	return b.String(), args
}

// nullable convierte la cadena vacía en NULL: en el agregado, «sin referente» y
// «referente vacío» no son la misma cosa.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
