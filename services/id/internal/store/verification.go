// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Para qué se emite un enlace de verificación (columna purpose, mig 0066).
const (
	// VerifyLogin confirma el correo de acceso de la cuenta.
	VerifyLogin = "login"
	// VerifyRecovery confirma el correo de recuperación pedido en «Mi cuenta».
	VerifyRecovery = "recovery"
)

// InsertEmailVerification persiste el hash de un token de verificación de email
// con la dirección que verifica y para qué. Antes borra los previos del mismo
// usuario y propósito: al pedir un enlace nuevo, los anteriores dejan de valer
// (solo el último). Debe ejecutarse en transacción (borrar + insertar) para que
// sea atómico. Mismo patrón que InsertResetToken.
func (s *Store) InsertEmailVerification(ctx context.Context, q pg.Querier, tokenHash, userID, email, purpose string, expires time.Time) error {
	if q == nil {
		q = s.DB()
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := q.Exec(ctx, `DELETE FROM email_verification_tokens WHERE user_id = $1 AND purpose = $2`,
		userID, purpose); err != nil {
		return errf("InsertEmailVerification.purge", err)
	}
	const sql = `INSERT INTO email_verification_tokens (token_hash, user_id, email, purpose, expires_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := q.Exec(ctx, sql, tokenHash, userID, email, purpose, expires)
	return errf("InsertEmailVerification", err)
}

// ConsumeEmailVerification recupera y BORRA un token de verificación válido
// (atómico, un-solo-uso, anti-replay), devolviendo el user_id, la dirección que
// verificaba y para qué. Un token caducado, ya usado o inexistente ⇒ ErrNotFound
// (indistinguibles para el llamante). Mismo patrón que ConsumeResetToken.
func (s *Store) ConsumeEmailVerification(ctx context.Context, q pg.Querier, tokenHash string, now time.Time) (userID, email, purpose string, err error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `DELETE FROM email_verification_tokens
		WHERE token_hash = $1 AND expires_at > $2
		RETURNING user_id::text, email, purpose`
	err = q.QueryRow(ctx, sql, tokenHash, now).Scan(&userID, &email, &purpose)
	return userID, email, purpose, errf("ConsumeEmailVerification", err)
}

// MarkEmailVerified marca email_verified=true PERO solo si la dirección sigue
// coincidiendo con el email actual del usuario: liga el token a la dirección para
// la que se emitió (si el correo cambió entre emitir y verificar, el enlace no
// aplica). Devuelve true si actualizó la fila. Comparación case-insensitive,
// coherente con el índice único users_email_lower_key.
func (s *Store) MarkEmailVerified(ctx context.Context, q pg.Querier, userID, email string) (bool, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx,
		`UPDATE users SET email_verified = true
			WHERE id = $1 AND lower(email) = lower($2) AND email_verified = false`,
		userID, strings.TrimSpace(email))
	if err != nil {
		return false, errf("MarkEmailVerified", err)
	}
	return tag.RowsAffected() > 0, nil
}
