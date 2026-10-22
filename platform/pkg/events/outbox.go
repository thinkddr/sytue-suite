// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// OutboxTableDDL es el DDL de la tabla outbox compartida. Los servicios pueden
// incluirlo en una migración propia (cada servicio gestiona su esquema).
const OutboxTableDDL = `
CREATE TABLE IF NOT EXISTS events_outbox (
    id           uuid PRIMARY KEY,
    channel      text        NOT NULL,
    type         text        NOT NULL,
    tenant       text        NOT NULL DEFAULT '',
    payload      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    attempts     int         NOT NULL DEFAULT 0,
    last_error   text,
    -- Reserva del evento mientras un Dispatcher lo procesa. El Handler corre
    -- FUERA de la transacción, así que el lock de fila ya no sirve de exclusión
    -- mutua: es este lease el que evita que el poll de respaldo (u otra réplica)
    -- reclame un evento que se está procesando.
    locked_until timestamptz
);
CREATE INDEX IF NOT EXISTS events_outbox_unprocessed_idx
    ON events_outbox (created_at)
    WHERE processed_at IS NULL;
`

// Publish escribe el evento en el outbox usando el Querier dado (típicamente
// una pgx.Tx de la transacción de negocio) y emite NOTIFY en la misma conexión,
// de modo que el aviso solo viaja si la transacción hace commit.
//
// Para publicación transaccional: pasa la misma tx que ejecuta el cambio de
// negocio. Para publicación independiente: usa PublishTx sobre el pool.
func Publish(ctx context.Context, q pg.Querier, ev Event) error {
	if ev.ID == "" || ev.Channel == "" || ev.Type == "" {
		return fmt.Errorf("events: evento incompleto")
	}
	payload := ev.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	const insertSQL = `
		INSERT INTO events_outbox (id, channel, type, tenant, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := q.Exec(ctx, insertSQL,
		ev.ID, string(ev.Channel), ev.Type, ev.Tenant, payload, ev.CreatedAt,
	); err != nil {
		return fmt.Errorf("events: insertar en outbox: %w", err)
	}

	note := Notification{ID: ev.ID, Channel: ev.Channel, Type: ev.Type}
	noteJSON, err := json.Marshal(note)
	if err != nil {
		return fmt.Errorf("events: serializar notificación: %w", err)
	}
	// pg_notify es seguro frente a inyección (parámetros) y respeta el commit.
	if _, err := q.Exec(ctx, "SELECT pg_notify($1, $2)", ev.Channel.channelName(), string(noteJSON)); err != nil {
		return fmt.Errorf("events: pg_notify: %w", err)
	}
	return nil
}

// PublishTx publica un evento en su propia transacción sobre el pool.
func PublishTx(ctx context.Context, pool *pg.Pool, ev Event) error {
	return pool.InTx(ctx, func(tx pgx.Tx) error {
		return Publish(ctx, tx, ev)
	})
}

// Purge borra del outbox los eventos procesados antes de `processedBefore` y,
// procesados o no, los creados antes de `createdBefore`: un aviso que nadie
// despachó en ese plazo ya no sirve, y sin purga la tabla crece sin límite.
// Devuelve cuántos borró. Corre con el Querier dado; events_outbox no lleva RLS.
// ponytail: un solo DELETE; por lotes (como Docs) si el backlog llega a pesar.
func Purge(ctx context.Context, q pg.Querier, processedBefore, createdBefore time.Time) (int64, error) {
	tag, err := q.Exec(ctx, `
		DELETE FROM events_outbox
		 WHERE (processed_at IS NOT NULL AND processed_at < $1) OR created_at < $2`,
		processedBefore, createdBefore)
	if err != nil {
		return 0, fmt.Errorf("events: purgar outbox: %w", err)
	}
	return tag.RowsAffected(), nil
}
