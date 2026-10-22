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

// El avatar vive en su propia tabla (user_avatars), no en userCols: ver la
// migración 0067. GetUserByID/ListUsers/SCIM nunca lo necesitan y así no
// inflan su respuesta con hasta 256 KiB de data URI por usuario.

// GetAvatar devuelve el data URI del avatar propio y su marca de actualización
// (para el etag del byte-serving endpoint), o domain.ErrNotFound si no ha
// puesto ninguno.
func (s *Store) GetAvatar(ctx context.Context, userID string) (string, time.Time, error) {
	const sql = `SELECT data_uri, updated_at FROM user_avatars WHERE user_id = $1`
	var dataURI string
	var updatedAt time.Time
	err := s.DB().QueryRow(ctx, sql, userID).Scan(&dataURI, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, domain.ErrNotFound
	}
	if err != nil {
		return "", time.Time{}, errf("GetAvatar", err)
	}
	return dataURI, updatedAt, nil
}

// HasAvatar dice si el usuario tiene avatar propio y desde cuándo (para el
// `?v=` de caché del <img>), sin traerse el data URI entero. La usa el perfil
// (GET/PATCH /me) para decidir si enseña el botón «Cambiar» o «Añadir».
func (s *Store) HasAvatar(ctx context.Context, userID string) (bool, time.Time, error) {
	var updatedAt time.Time
	err := s.DB().QueryRow(ctx, `SELECT updated_at FROM user_avatars WHERE user_id = $1`, userID).
		Scan(&updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, errf("HasAvatar", err)
	}
	return true, updatedAt, nil
}

// SetAvatar guarda (o reemplaza) el avatar propio, ya validado por el llamante.
func (s *Store) SetAvatar(ctx context.Context, userID, dataURI string) (time.Time, error) {
	const sql = `INSERT INTO user_avatars (user_id, data_uri, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (user_id) DO UPDATE SET data_uri = excluded.data_uri, updated_at = excluded.updated_at
		RETURNING updated_at`
	var updatedAt time.Time
	err := s.DB().QueryRow(ctx, sql, userID, dataURI).Scan(&updatedAt)
	return updatedAt, errf("SetAvatar", err)
}

// ClearAvatar quita el avatar propio (vuelve a las iniciales). No falla si no
// había ninguno: quitar lo que ya está quitado es un no-op, no un error.
func (s *Store) ClearAvatar(ctx context.Context, userID string) error {
	_, err := s.DB().Exec(ctx, `DELETE FROM user_avatars WHERE user_id = $1`, userID)
	return errf("ClearAvatar", err)
}
