// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// AppPassword es una contraseña de aplicación de UNA persona (mig 0056): la
// credencial con la que su cliente de correo entra por IMAP/SMTP. El secreto no
// está aquí, solo su hash; se enseña una vez al crearla.
type AppPassword struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// CreateAppPassword guarda el hash de una contraseña nueva y devuelve la fila.
func (s *Store) CreateAppPassword(ctx context.Context, userID, secretHash, label string) (AppPassword, error) {
	const sql = `INSERT INTO app_passwords (user_id, secret_hash, label)
		VALUES ($1::uuid, $2, $3)
		RETURNING id, label, created_at, last_used_at`
	var a AppPassword
	err := s.DB().QueryRow(ctx, sql, userID, secretHash, label).
		Scan(&a.ID, &a.Label, &a.CreatedAt, &a.LastUsedAt)
	return a, errf("CreateAppPassword", err)
}

// ListAppPasswords devuelve las VIVAS de una persona, la más reciente primero.
func (s *Store) ListAppPasswords(ctx context.Context, userID string) ([]AppPassword, error) {
	const sql = `SELECT id, label, created_at, last_used_at
		FROM app_passwords WHERE user_id = $1::uuid AND revoked_at IS NULL
		ORDER BY created_at DESC`
	rows, err := s.DB().Query(ctx, sql, userID)
	if err != nil {
		return nil, errf("ListAppPasswords", err)
	}
	defer rows.Close()
	out := []AppPassword{}
	for rows.Next() {
		var a AppPassword
		if err := rows.Scan(&a.ID, &a.Label, &a.CreatedAt, &a.LastUsedAt); err != nil {
			return nil, errf("ListAppPasswords.scan", err)
		}
		out = append(out, a)
	}
	return out, errf("ListAppPasswords.iter", rows.Err())
}

// RevokeAppPassword revoca una contraseña PROPIA. El `user_id` del WHERE es la
// verja: sin él, un id ajeno adivinado revocaría la credencial de otra persona.
// Devuelve false si no había ninguna viva con ese id para ese usuario.
func (s *Store) RevokeAppPassword(ctx context.Context, userID, id string) (bool, error) {
	const sql = `UPDATE app_passwords SET revoked_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND revoked_at IS NULL`
	tag, err := s.DB().Exec(ctx, sql, id, userID)
	if err != nil {
		return false, errf("RevokeAppPassword", err)
	}
	return tag.RowsAffected() > 0, nil
}

// UserByAppPassword resuelve la persona a la que pertenece una contraseña de
// aplicación viva y ANOTA el uso, todo en la misma consulta: el camino de
// autenticación de cada conexión IMAP/SMTP no puede costar dos viajes.
//
// La búsqueda es por el hash, que es único y está indexado por la restricción de
// unicidad: no se recorre nada. Una revocada no casa (revoked_at IS NULL), así
// que revocar corta en la siguiente conexión sin desplegar ni reiniciar nada.
func (s *Store) UserByAppPassword(ctx context.Context, secretHash string) (domain.User, error) {
	const sql = `UPDATE app_passwords SET last_used_at = now()
		WHERE secret_hash = $1 AND revoked_at IS NULL
		RETURNING user_id`
	var userID string
	if err := s.DB().QueryRow(ctx, sql, secretHash).Scan(&userID); err != nil {
		return domain.User{}, errf("UserByAppPassword", err)
	}
	return s.GetUserByID(ctx, userID)
}

// CreateAppleProfileLink guarda un enlace de un solo uso al perfil de Apple (mig 0118).
// Aprovecha para barrer los caducados: son filas de minutos y nadie más las borra.
func (s *Store) CreateAppleProfileLink(ctx context.Context, tokenHash, userID, tenant, label string, caduca time.Time) error {
	if _, err := s.DB().Exec(ctx, `DELETE FROM apple_profile_links WHERE expires_at < now() - interval '1 day'`); err != nil {
		return errf("CreateAppleProfileLink.barrer", err)
	}
	const sql = `INSERT INTO apple_profile_links (token_hash, user_id, tenant, label, expires_at)
		VALUES ($1, $2::uuid, $3, $4, $5)`
	_, err := s.DB().Exec(ctx, sql, tokenHash, userID, tenant, label, caduca)
	return errf("CreateAppleProfileLink", err)
}

// ConsumeAppleProfileLink canjea el enlace: lo marca usado y devuelve de quién es, en la
// MISMA consulta, así que dos descargas a la vez no se llevan dos perfiles. Uno usado,
// caducado o desconocido es domain.ErrNotFound.
func (s *Store) ConsumeAppleProfileLink(ctx context.Context, tokenHash string) (userID, tenant, label string, err error) {
	const sql = `UPDATE apple_profile_links SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id, tenant, label`
	err = s.DB().QueryRow(ctx, sql, tokenHash).Scan(&userID, &tenant, &label)
	return userID, tenant, label, errf("ConsumeAppleProfileLink", err)
}
