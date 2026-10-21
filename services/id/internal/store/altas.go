// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Altas a medio hacer (signup_phone_codes, mig 0066): lo que la persona rellenó
// mientras confirma el código que se le mandó por SMS. La cuenta nace al
// confirmarlo, no antes.

const signupCodeCols = `id::text, phone, username, name, password_hash, code_hash, attempts, sent_at, expires_at`

func scanSignupCode(row interface{ Scan(...any) error }) (domain.SignupCode, error) {
	var c domain.SignupCode
	err := row.Scan(&c.ID, &c.Phone, &c.Username, &c.Name, &c.PasswordHash, &c.CodeHash,
		&c.Attempts, &c.SentAt, &c.ExpiresAt)
	return c, err
}

// CreateSignupCode guarda un alta pendiente con el hash de su código.
func (s *Store) CreateSignupCode(ctx context.Context, c domain.SignupCode) error {
	_, err := s.DB().Exec(ctx, `INSERT INTO signup_phone_codes
		(id, phone, username, name, password_hash, code_hash, sent_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		c.ID, c.Phone, c.Username, c.Name, c.PasswordHash, c.CodeHash, c.SentAt, c.ExpiresAt)
	return errf("CreateSignupCode", err)
}

// GetSignupCode lee un alta pendiente que no ha caducado. ErrNotFound si no.
func (s *Store) GetSignupCode(ctx context.Context, id string, now time.Time) (domain.SignupCode, error) {
	c, err := scanSignupCode(s.DB().QueryRow(ctx,
		`SELECT `+signupCodeCols+` FROM signup_phone_codes WHERE id = $1 AND expires_at > $2`, id, now))
	return c, errf("GetSignupCode", err)
}

// TrySignupCode GASTA un intento y devuelve el alta para comparar el código.
// ErrNotFound si no existe, caducó o ya no le quedan intentos: para quien llama,
// las tres son «vuelve a empezar». El intento se gasta antes de comparar y en la
// misma sentencia que comprueba el tope, así que dos envíos a la vez no prueban
// más códigos de los permitidos.
func (s *Store) TrySignupCode(ctx context.Context, id string, now time.Time) (domain.SignupCode, error) {
	c, err := scanSignupCode(s.DB().QueryRow(ctx, `UPDATE signup_phone_codes SET attempts = attempts + 1
		WHERE id = $1 AND expires_at > $2 AND attempts < $3
		RETURNING `+signupCodeCols, id, now, domain.SignupCodeMaxAttempts))
	return c, errf("TrySignupCode", err)
}

// RenewSignupCode pone un código nuevo a un alta vigente (reenvío): intentos a
// cero y plazo desde ahora. Solo si el anterior se mandó hace al menos
// SignupCodeResendAfter; si no, o si el alta ya no existe, ErrNotFound.
func (s *Store) RenewSignupCode(ctx context.Context, id, codeHash string, now time.Time) error {
	tag, err := s.DB().Exec(ctx, `UPDATE signup_phone_codes
		SET code_hash = $2, attempts = 0, sent_at = $3, expires_at = $4
		WHERE id = $1 AND expires_at > $3 AND sent_at <= $5`,
		id, codeHash, now, now.Add(domain.SignupCodeTTL), now.Add(-domain.SignupCodeResendAfter))
	if err != nil {
		return errf("RenewSignupCode", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteSignupCode borra un alta pendiente: terminada, o que ya no puede terminar.
func (s *Store) DeleteSignupCode(ctx context.Context, id string) error {
	_, err := s.DB().Exec(ctx, `DELETE FROM signup_phone_codes WHERE id = $1`, id)
	return errf("DeleteSignupCode", err)
}
