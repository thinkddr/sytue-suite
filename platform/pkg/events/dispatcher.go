// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package events

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// DispatcherConfig configura el relay del outbox.
type DispatcherConfig struct {
	// Channels son los canales lógicos en los que escuchar (LISTEN).
	Channels []Channel
	// PollInterval es el periodo de poll de respaldo del outbox (por si se
	// pierde un NOTIFY). Por defecto 5s.
	PollInterval time.Duration
	// BatchSize es el máximo de eventos no procesados a leer por ciclo.
	BatchSize int
	// MaxAttempts es el número máximo de reintentos por evento antes de marcarlo
	// como fallido permanente (last_error queda registrado). Por defecto 10;
	// un valor negativo conserva los eventos hasta poder entregarlos.
	MaxAttempts int
	// LeaseDuration es cuánto tiempo queda reservado un evento reclamado antes de
	// volver a estar disponible. Debe superar lo que tarde el Handler más lento:
	// si expira mientras se procesa, el siguiente poll lo reclama otra vez y el
	// efecto se repite (at-least-once). Por defecto 2 min.
	LeaseDuration time.Duration
	// Types, si no está vacía, limita el Dispatcher a esos tipos de evento. Sirve
	// para dar a un trabajo largo su propia cola: con una sola, una migración de
	// horas dejaba esperando a todo lo demás del servicio (NEG-20). Vacía, como
	// hasta ahora: todos.
	Types []string
}

// Dispatcher entrega eventos del outbox a un Handler con semántica
// at-least-once. Combina LISTEN/NOTIFY (latencia baja) con un poll de respaldo
// (robustez ante NOTIFY perdidos o caídas de conexión).
type Dispatcher struct {
	pool    *pg.Pool
	handler Handler
	cfg     DispatcherConfig
}

// NewDispatcher construye un Dispatcher.
func NewDispatcher(pool *pg.Pool, handler Handler, cfg DispatcherConfig) *Dispatcher {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 10
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 2 * time.Minute
	}
	return &Dispatcher{pool: pool, handler: handler, cfg: cfg}
}

// Run bloquea entregando eventos hasta que el contexto se cancele. Mantiene una
// conexión dedicada con LISTEN y reabre con backoff si se pierde.
func (d *Dispatcher) Run(ctx context.Context) error {
	l := log.From(ctx).With("component", "events.dispatcher")
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // cancelación del contexto = apagado ordenado, no un fallo.
		}
		if err := d.listenLoop(ctx); err != nil && !errors.Is(err, context.Canceled) {
			l.Warn("bucle LISTEN interrumpido, reintentando", "error", err, "backoff", backoff.String())
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		return nil
	}
}

// listenLoop adquiere una conexión dedicada, ejecuta LISTEN en cada canal y
// procesa NOTIFY más un poll periódico de respaldo.
func (d *Dispatcher) listenLoop(ctx context.Context) error {
	// Conexión DIRECTA a PostgreSQL: LISTEN es estado de sesión y no funciona a
	// través de PgBouncer en pool_mode=transaction (prod). En dev es el pool normal.
	conn, err := d.pool.AcquireDirect(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	for _, ch := range d.cfg.Channels {
		// El nombre de canal proviene de constantes internas (no input externo);
		// se cita con identificador para LISTEN.
		if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{ch.channelName()}.Sanitize()); err != nil {
			return err
		}
	}

	// Drenaje inicial: procesa lo que ya hubiera pendiente.
	if err := d.drain(ctx); err != nil {
		log.From(ctx).Error("drenaje inicial del outbox falló", "error", err)
	}

	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		waitCtx, cancel := context.WithTimeout(ctx, d.cfg.PollInterval)
		_, err := conn.Conn().WaitForNotification(waitCtx)
		cancel()

		switch {
		case err == nil:
			// NOTIFY recibido: drena el outbox.
		case errors.Is(err, context.DeadlineExceeded):
			// Timeout del wait: cae al poll de respaldo de abajo.
		case errors.Is(err, context.Canceled) || ctx.Err() != nil:
			return nil
		default:
			return err
		}

		if err := d.drain(ctx); err != nil {
			log.From(ctx).Error("drenaje del outbox falló", "error", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := d.drain(ctx); err != nil {
				log.From(ctx).Error("poll del outbox falló", "error", err)
			}
		default:
		}
	}
}

// drain procesa lotes de eventos no procesados hasta vaciar el outbox. Usa
// FOR UPDATE SKIP LOCKED para permitir múltiples relays concurrentes.
func (d *Dispatcher) drain(ctx context.Context) error {
	for {
		n, err := d.drainBatch(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

// drainBatch reclama un lote y lo procesa en TRES pasos, con el Handler FUERA de
// toda transacción:
//
//  1. claim (tx corta): FOR UPDATE SKIP LOCKED + reserva por `locked_until`.
//  2. handler: sin transacción ni conexión retenida.
//  3. settle (tx corta por evento): processed_at, o libera el lease con el error.
//
// El Handler ya NO corre dentro de la tx del lote. Hacerlo era un deadlock de
// pool esperando a ocurrir: la tx retiene una conexión y el handler pide otra
// (los stores abren su propia InTenantTx), de modo que con el pool al límite
// nadie avanza, la tx queda «idle in transaction» y el servicio deja de
// responder. Además el lote sostenía locks de fila durante el I/O de red del
// handler (webhooks salientes), lo que ninguna cola debería hacer.
//
// El lease es lo que sustituye al lock de fila como exclusión mutua: sin él, el
// poll de respaldo (PollInterval) volvería a reclamar el evento mientras el
// handler sigue trabajando y repetiría el efecto — con UNA sola réplica.
func (d *Dispatcher) drainBatch(ctx context.Context) (int, error) {
	// `attempts` se incrementa AL RECLAMAR, no al fallar: así un evento que
	// tumba al proceso a mitad de proceso consume intentos y acaba parando en
	// MaxAttempts, en vez de reintentarse para siempre.
	const claimSQL = `
		UPDATE events_outbox
		   SET attempts = attempts + 1,
		       locked_until = now() + make_interval(secs => $3)
		 WHERE id IN (
		       SELECT id FROM events_outbox
		        WHERE processed_at IS NULL
		          AND ($1 < 0 OR attempts < $1)
		          AND (locked_until IS NULL OR locked_until < now())
		          AND (cardinality($4::text[]) = 0 OR type = ANY($4::text[]))
		        ORDER BY created_at
		        FOR UPDATE SKIP LOCKED
		        LIMIT $2)
	 RETURNING id, channel, type, tenant, payload, created_at`

	var batch []Event
	err := d.pool.InTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, claimSQL,
			d.cfg.MaxAttempts, d.cfg.BatchSize, d.cfg.LeaseDuration.Seconds(), append([]string{}, d.cfg.Types...))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ev Event
			var channel string
			if err := rows.Scan(&ev.ID, &channel, &ev.Type, &ev.Tenant, &ev.Payload, &ev.CreatedAt); err != nil {
				return err
			}
			ev.Channel = Channel(channel)
			batch = append(batch, ev)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}

	var processed int
	for _, ev := range batch {
		hErr := d.handler(ctx, ev)
		if sErr := d.settle(ctx, ev, hErr); sErr != nil {
			return processed, sErr
		}
		if hErr == nil {
			processed++
		}
	}
	return processed, nil
}

// settle cierra un evento ya procesado: lo marca como hecho, o libera su lease
// dejando el error para que el siguiente ciclo lo reintente (hasta MaxAttempts).
func (d *Dispatcher) settle(ctx context.Context, ev Event, hErr error) error {
	if hErr != nil {
		_, err := d.pool.Exec(ctx,
			`UPDATE events_outbox SET last_error = $2, locked_until = NULL WHERE id = $1`,
			ev.ID, hErr.Error())
		return err
	}
	_, err := d.pool.Exec(ctx,
		`UPDATE events_outbox
		    SET processed_at = now(), last_error = NULL, locked_until = NULL
		  WHERE id = $1`, ev.ID)
	return err
}
