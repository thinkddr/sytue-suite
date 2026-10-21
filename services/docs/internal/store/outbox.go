// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// purgeBatch limita cada DELETE de la purga del outbox para no retener locks ni
// generar transacciones enormes cuando hay backlog acumulado.
const purgeBatch = 5000

// PurgeOutboxBefore borra de events_outbox los eventos creados antes del
// instante dado, por lotes, hasta agotar el backlog o cancelarse el contexto.
// Devuelve el total de filas eliminadas.
//
// El outbox de docs es fan-out efímero (NOTIFY + events.GetByID): la fila solo
// se necesita unos segundos tras publicarse; nadie marca processed_at, así que
// sin purga la tabla crecería sin límite. events_outbox no tiene RLS (no es
// dato de tenant), por lo que la purga corre en una transacción de servicio.
// Usa events_outbox_created_idx (migración 0002) — sin seq-scans.
func (s *Store) PurgeOutboxBefore(ctx context.Context, before time.Time) (int64, error) {
	var total int64
	for {
		var n int64
		err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `
				DELETE FROM events_outbox
				WHERE id IN (
					SELECT id FROM events_outbox
					WHERE created_at < $1
					LIMIT $2
				)`, before, purgeBatch)
			if err != nil {
				return err
			}
			n = tag.RowsAffected()
			return nil
		})
		if err != nil {
			return total, fmt.Errorf("store: purgar outbox: %w", err)
		}
		total += n
		if n < purgeBatch {
			return total, nil
		}
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
	}
}
