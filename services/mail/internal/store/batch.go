// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Operaciones en LOTE sobre mensajes (multiselección de la lista → una acción
// aplicada a N mensajes). Todas son set-based (id = ANY, sin N round-trips) y
// owner-scoped por account_id: un cliente solo afecta a mensajes de su cuenta
// (los ids ajenos NO casan y se omiten), además del aislamiento por tenant (RLS).
// Tocan updated_at para que el delta incremental (Changes) refleje el cambio.

// MoveMessages mueve un lote de mensajes a otro buzón en UNA operación,
// asignando UIDs IMAP consecutivos en el destino con una función de ventana
// (row_number), sin un round-trip por mensaje. Devuelve cuántos se movieron.
// Seguridad: el buzón destino debe ser de la cuenta (no solo del tenant), para
// que un usuario no pueda mover sus mensajes al buzón de otro usuario del mismo
// tenant. Excluye los que ya están en el destino (no malgasta UIDs; idempotente).
func (s *Store) MoveMessages(ctx context.Context, accountID uuid.UUID, ids []uuid.UUID, dest uuid.UUID) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var moved int
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var owned bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM mailboxes WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $2)`,
			dest, accountID).Scan(&owned); err != nil {
			return fmt.Errorf("store: verificar buzón destino: %w", err)
		}
		if !owned {
			return domain.ErrNotFound
		}
		// Un solo statement: numera los mensajes a mover (owner-scoped, excluyendo
		// los ya-en-destino), reserva ese bloque de UIDs en el destino y los reasigna
		// consecutivos. Los CTE que modifican datos se materializan una vez, así que
		// `moving` es el mismo conjunto para el conteo, la reserva y la reasignación.
		rows, err := tx.Query(ctx, `
			WITH moving AS (
				SELECT id, row_number() OVER (ORDER BY received_at, id) - 1 AS rn
				FROM messages
				WHERE tenant = current_setting('app.tenant', true) AND account_id = $2 AND id = ANY($3)
				  AND deleted_at IS NULL AND mailbox_id <> $1
			),
			reserve AS (
				UPDATE mailboxes
				SET uid_next = uid_next + (SELECT count(*) FROM moving), updated_at = now()
				WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND (SELECT count(*) FROM moving) > 0
				RETURNING uid_next - (SELECT count(*) FROM moving) AS base
			)
			UPDATE messages m
			SET mailbox_id = $1,
			    uid = (SELECT base FROM reserve) + moving.rn,
			    updated_at = now()
			FROM moving
			WHERE m.tenant = current_setting('app.tenant', true) AND m.id = moving.id
			RETURNING m.id`, dest, accountID, ids)
		if err != nil {
			return fmt.Errorf("store: mover lote: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			moved++
		}
		return rows.Err()
	})
	return moved, err
}

// SetFlagsBatch añade (add=true) o quita (add=false) un flag/keyword a un lote de
// mensajes en un solo UPDATE (id = ANY, sin N round-trips). Owner-scoped por
// account_id (ids ajenos se omiten) + RLS por tenant. Añadir es idempotente
// (array_remove + append deduplica). Devuelve cuántos se afectaron.
func (s *Store) SetFlagsBatch(ctx context.Context, accountID uuid.UUID, ids []uuid.UUID, flag string, add bool) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	q := `UPDATE messages SET flags = array_append(array_remove(flags, $1), $1), updated_at = now()
	      WHERE tenant = current_setting('app.tenant', true) AND account_id = $2 AND id = ANY($3) AND deleted_at IS NULL`
	if !add {
		q = `UPDATE messages SET flags = array_remove(flags, $1), updated_at = now()
		     WHERE tenant = current_setting('app.tenant', true) AND account_id = $2 AND id = ANY($3) AND deleted_at IS NULL`
	}
	var n int
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, q, flag, accountID, ids)
		if err != nil {
			return fmt.Errorf("store: set flags lote: %w", err)
		}
		n = int(tag.RowsAffected())
		return nil
	})
	return n, err
}
