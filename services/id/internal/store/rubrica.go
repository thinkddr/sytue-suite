// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// La rúbrica guardada (migración 0120) se guarda cifrada con la misma KEK que el
// resto del material sensible del servicio. El llamante entrega y recibe el JSON
// de los trazos en claro, ya validado: el cifrado es asunto del almacén.

// GetRubrica devuelve los trazos guardados (JSON en claro) y cuándo se guardaron,
// o domain.ErrNotFound si la persona no pidió recordar ninguna.
func (s *Store) GetRubrica(ctx context.Context, userID string) (string, time.Time, error) {
	var cifrado string
	var updatedAt time.Time
	err := s.DB().QueryRow(ctx,
		`SELECT trazos_cifrados, updated_at FROM user_rubricas WHERE user_id = $1`, userID).
		Scan(&cifrado, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, domain.ErrNotFound
	}
	if err != nil {
		return "", time.Time{}, errf("GetRubrica", err)
	}
	claro, err := s.keyCipher.Decrypt(cifrado)
	if err != nil {
		return "", time.Time{}, errf("GetRubrica", err)
	}
	return claro, updatedAt, nil
}

// SetRubrica guarda (o reemplaza) la rúbrica de la persona.
func (s *Store) SetRubrica(ctx context.Context, userID, trazosJSON string) error {
	cifrado, err := s.keyCipher.Encrypt(trazosJSON)
	if err != nil {
		return errf("SetRubrica", err)
	}
	_, err = s.DB().Exec(ctx, `INSERT INTO user_rubricas (user_id, trazos_cifrados, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (user_id) DO UPDATE SET trazos_cifrados = excluded.trazos_cifrados, updated_at = excluded.updated_at`,
		userID, cifrado)
	return errf("SetRubrica", err)
}

// ClearRubrica olvida la rúbrica guardada. Olvidar lo que no estaba es un no-op.
func (s *Store) ClearRubrica(ctx context.Context, userID string) error {
	_, err := s.DB().Exec(ctx, `DELETE FROM user_rubricas WHERE user_id = $1`, userID)
	return errf("ClearRubrica", err)
}
