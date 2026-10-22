// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package events

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// NotificationHandler procesa la notificación ligera de un NOTIFY. Útil para
// casos efímeros (presencia, "está escribiendo...") donde NO se requiere la
// durabilidad del outbox; para entrega garantizada usa Dispatcher.
type NotificationHandler func(ctx context.Context, ch Channel, n Notification)

// Notification es la carga ligera transportada por un NOTIFY.
type Notification struct {
	ID      string  `json:"id"`
	Channel Channel `json:"channel"`
	Type    string  `json:"type"`
	// Data es la carga de un aviso EFÍMERO (Notificar), sin fila en el outbox: lo
	// que se reparte entre réplicas y no hace falta recuperar si se pierde. Texto
	// (JSON serializado) y no RawMessage para que Notification siga siendo comparable.
	Data string `json:"data,omitempty"`
}

// maxNotify es el tope de la carga de un NOTIFY (8000 bytes en PostgreSQL), con
// margen para el sobre.
const maxNotify = 7800

// ErrDemasiadoGrande: la carga no cabe en un NOTIFY. Quien llama decide qué hacer
// (lo normal: quedarse en su réplica).
var ErrDemasiadoGrande = errors.New("events: la carga no cabe en un NOTIFY")

// Notificar manda un aviso efímero por LISTEN/NOTIFY, sin outbox: le llega a
// cada réplica que escuche el canal con un Subscriber, también a la que lo manda.
func Notificar(ctx context.Context, q pg.Querier, n Notification) error {
	b, err := json.Marshal(n)
	if err != nil {
		return err
	}
	if len(b) > maxNotify {
		return ErrDemasiadoGrande
	}
	_, err = q.Exec(ctx, "SELECT pg_notify($1, $2)", n.Channel.channelName(), string(b))
	return err
}

// Subscriber escucha canales LISTEN/NOTIFY de PostgreSQL en tiempo real.
type Subscriber struct {
	pool     *pg.Pool
	channels []Channel
	handler  NotificationHandler
}

// NewSubscriber construye un Subscriber sobre los canales dados.
func NewSubscriber(pool *pg.Pool, handler NotificationHandler, channels ...Channel) *Subscriber {
	return &Subscriber{pool: pool, channels: channels, handler: handler}
}

// Run bloquea escuchando NOTIFY hasta que se cancele el contexto. Reabre la
// conexión con backoff si se pierde.
func (s *Subscriber) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // cancelación del contexto = apagado ordenado, no un fallo.
		}
		err := s.listen(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}
		log.From(ctx).Warn("subscriber: conexión LISTEN perdida, reintentando",
			"error", err, "backoff", backoff.String())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (s *Subscriber) listen(ctx context.Context) error {
	// Conexión DIRECTA a PostgreSQL: LISTEN es estado de sesión y no funciona a
	// través de PgBouncer en pool_mode=transaction (prod). En dev es el pool normal.
	conn, err := s.pool.AcquireDirect(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	for _, ch := range s.channels {
		if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{ch.channelName()}.Sanitize()); err != nil {
			return err
		}
	}

	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		ch := channelFromPG(n.Channel)
		var note Notification
		if err := json.Unmarshal([]byte(n.Payload), &note); err != nil {
			log.From(ctx).Warn("subscriber: payload NOTIFY inválido", "channel", n.Channel, "error", err)
			continue
		}
		note.Channel = ch
		s.handler(ctx, ch, note)
	}
}

// GetByID lee un evento del outbox por su ID (el que viaja en la Notification de
// un NOTIFY). Sirve para el fan-out efímero por réplica: cada réplica recibe el
// NOTIFY (LISTEN es broadcast) y recupera aquí la carga completa para entregarla
// a sus clientes locales, SIN marcar processed_at (eso es exclusivo del
// Dispatcher, que garantiza ejecución única a nivel de clúster). Devuelve
// (Event{}, false, nil) si la fila no existe (p. ej. purgada).
func GetByID(ctx context.Context, pool *pg.Pool, id string) (Event, bool, error) {
	const sql = `SELECT id, channel, type, tenant, payload, created_at
		FROM events_outbox WHERE id = $1`
	var ev Event
	var channel string
	err := pool.QueryRow(ctx, sql, id).Scan(
		&ev.ID, &channel, &ev.Type, &ev.Tenant, &ev.Payload, &ev.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Event{}, false, nil
		}
		return Event{}, false, err
	}
	ev.Channel = Channel(channel)
	return ev, true, nil
}

// channelFromPG revierte el prefijo "sytue_" del nombre de canal Postgres.
func channelFromPG(name string) Channel {
	const prefix = "sytue_"
	if len(name) > len(prefix) && name[:len(prefix)] == prefix {
		return Channel(name[len(prefix):])
	}
	return Channel(name)
}
