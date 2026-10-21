// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// InsertResetToken persiste el hash de un token de reseteo de contraseña. Antes
// borra los tokens previos del mismo usuario: al pedir un enlace nuevo, los
// anteriores dejan de valer (solo el último). Debe ejecutarse en transacción
// (borrar + insertar) para que sea atómico.
func (s *Store) InsertResetToken(ctx context.Context, q pg.Querier, tokenHash, userID string, expires time.Time) error {
	if q == nil {
		q = s.DB()
	}
	if _, err := q.Exec(ctx, `DELETE FROM password_reset_tokens WHERE user_id = $1`, userID); err != nil {
		return errf("InsertResetToken.purge", err)
	}
	const sql = `INSERT INTO password_reset_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)`
	_, err := q.Exec(ctx, sql, tokenHash, userID, expires)
	return errf("InsertResetToken", err)
}

// ConsumeResetToken recupera y BORRA un token de reseteo válido (atómico,
// un-solo-uso, anti-replay), devolviendo el user_id. Un token caducado, ya usado
// o inexistente ⇒ ErrNotFound (indistinguibles para el llamante).
func (s *Store) ConsumeResetToken(ctx context.Context, q pg.Querier, tokenHash string, now time.Time) (userID string, err error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `DELETE FROM password_reset_tokens
		WHERE token_hash = $1 AND expires_at > $2
		RETURNING user_id::text`
	err = q.QueryRow(ctx, sql, tokenHash, now).Scan(&userID)
	return userID, errf("ConsumeResetToken", err)
}
