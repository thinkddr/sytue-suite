// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// MatrixRecoveryKey devuelve la clave de recuperación custodiada de la persona
// (ADR 0081), o domain.ErrNotFound si no hay ninguna. La tabla solo se ve en
// mantenimiento: quien llama ya ha comprobado que `subject` es quien pregunta.
func (s *Store) MatrixRecoveryKey(ctx context.Context, subject string) (string, error) {
	if !s.cipher.Enabled() {
		return "", ErrNoCipher
	}
	var sealed string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT sealed FROM matrix_recovery_keys WHERE subject=$1`, subject).Scan(&sealed)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return s.cipher.Decrypt(sealed)
}

// PutMatrixRecoveryKey guarda (o sustituye, tras restablecer el cifrado) la clave
// de recuperación de la persona, cifrada con la KEK. Sin KEK falla cerrado.
func (s *Store) PutMatrixRecoveryKey(ctx context.Context, subject, key string) error {
	if key == "" {
		return domain.ErrInvalidInput
	}
	sealed, err := s.sealSecret(key)
	if err != nil {
		return err
	}
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO matrix_recovery_keys(subject,sealed) VALUES($1,$2)
			ON CONFLICT(subject) DO UPDATE SET sealed=EXCLUDED.sealed, updated_at=now()`, subject, sealed)
		return err
	})
}
