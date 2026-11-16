// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Ventana de deshacer el envío (columna accounts.undo_send_seconds, migración
// 0020). Mismo patrón que la firma: un escalar de la cuenta, RLS por tenant más el
// id de cuenta que resuelve el token.

// GetUndoSendSeconds devuelve la ventana de deshacer de la cuenta, en segundos.
// Cuenta inexistente ⇒ ErrNotFound.
func (s *Store) GetUndoSendSeconds(ctx context.Context, accountID uuid.UUID) (int, error) {
	var secs int
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT undo_send_seconds FROM accounts WHERE tenant = current_setting('app.tenant', true) AND id = $1`, accountID).Scan(&secs)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: get undo send: %w", err)
	}
	return secs, nil
}

// SetUndoSendSeconds guarda la ventana de deshacer. El valor ya viene validado
// por el handler; el CHECK de la columna es la red de abajo. Ninguna fila tocada
// ⇒ la cuenta no es de este tenant ⇒ ErrNotFound, no un éxito silencioso.
func (s *Store) SetUndoSendSeconds(ctx context.Context, accountID uuid.UUID, secs int) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE accounts SET undo_send_seconds = $2, updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1`,
			accountID, secs)
		if err != nil {
			return fmt.Errorf("store: set undo send: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
