// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Cambiar el móvil y el @usuario desde Mi cuenta (mig 0126; ADR 0040 y 0041).

// ReservaUsername es lo que dura la reserva del @usuario que se deja, y también cada
// cuánto se puede cambiar.
const ReservaUsername = 30 * 24 * time.Hour

// CambioDeMovil es el móvil nuevo pendiente de confirmar con el código del SMS.
type CambioDeMovil struct {
	UserID, Phone, CodeHash string
	Attempts                int
	SentAt, ExpiresAt       time.Time
}

const cambioMovilCols = `user_id::text, phone, code_hash, attempts, sent_at, expires_at`

func scanCambioDeMovil(row pgx.Row) (CambioDeMovil, error) {
	var c CambioDeMovil
	err := row.Scan(&c.UserID, &c.Phone, &c.CodeHash, &c.Attempts, &c.SentAt, &c.ExpiresAt)
	return c, err
}

// PedirCambioDeMovil guarda el código de un móvil nuevo y sustituye al que hubiera
// pendiente, salvo que se mandara hace menos de SignupCodeResendAfter: entonces
// ErrConflict y no se manda otro SMS.
func (s *Store) PedirCambioDeMovil(ctx context.Context, userID, phone, codeHash string, now time.Time) error {
	tag, err := s.DB().Exec(ctx, `INSERT INTO phone_change_codes
		(user_id, phone, code_hash, attempts, sent_at, expires_at)
		VALUES ($1, $2, $3, 0, $4, $5)
		ON CONFLICT (user_id) DO UPDATE
		SET phone = EXCLUDED.phone, code_hash = EXCLUDED.code_hash, attempts = 0,
		    sent_at = EXCLUDED.sent_at, expires_at = EXCLUDED.expires_at
		WHERE phone_change_codes.sent_at <= $6`,
		userID, phone, codeHash, now, now.Add(domain.SignupCodeTTL), now.Add(-domain.SignupCodeResendAfter))
	if err != nil {
		return errf("PedirCambioDeMovil", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// ProbarCambioDeMovil GASTA un intento y devuelve el cambio pendiente para comparar
// el código. ErrNotFound si no hay, caducó o ya no le quedan intentos. Como en el
// alta, el intento se gasta antes de comparar y en la sentencia que mira el tope.
func (s *Store) ProbarCambioDeMovil(ctx context.Context, userID string, now time.Time) (CambioDeMovil, error) {
	c, err := scanCambioDeMovil(s.DB().QueryRow(ctx, `UPDATE phone_change_codes SET attempts = attempts + 1
		WHERE user_id = $1 AND expires_at > $2 AND attempts < $3
		RETURNING `+cambioMovilCols, userID, now, domain.SignupCodeMaxAttempts))
	return c, errf("ProbarCambioDeMovil", err)
}

// OlvidarCambioDeMovil borra el cambio pendiente: hecho, o que ya no puede hacerse.
func (s *Store) OlvidarCambioDeMovil(ctx context.Context, userID string) error {
	_, err := s.DB().Exec(ctx, `DELETE FROM phone_change_codes WHERE user_id = $1`, userID)
	return errf("OlvidarCambioDeMovil", err)
}

// SetPhone fija el móvil ya verificado. ErrConflict si es el de otra cuenta.
func (s *Store) SetPhone(ctx context.Context, userID, phone string) error {
	_, err := s.DB().Exec(ctx, `UPDATE users SET phone = $2, updated_at = now() WHERE id = $1`, userID, phone)
	return errf("SetPhone", err)
}

// ErrCambioDeUsernameReciente: el @usuario se cambió hace menos de ReservaUsername.
var ErrCambioDeUsernameReciente = errors.New("store: el @usuario se cambió hace poco")

// UsernameLibrePara dice si una persona puede quedarse con un @usuario: no es el de
// otra persona ni la parte local de una dirección AJENA del dominio raíz —las suyas no
// le estorban— ni está reservado para quien lo dejó hace menos de ReservaUsername.
func (s *Store) UsernameLibrePara(ctx context.Context, userID, username string) (bool, error) {
	var libre bool
	dir := strings.ToLower(username) + "@" + strings.ToLower(s.suiteDomain)
	err := s.DB().QueryRow(ctx, `SELECT
		NOT EXISTS (SELECT 1 FROM users WHERE id <> $3 AND (lower(username) = lower($1) OR lower(email) = $2))
		AND NOT EXISTS (SELECT 1 FROM org_mailboxes WHERE lower(address) = $2 AND removed_at IS NULL AND user_id <> $3)
		AND NOT EXISTS (SELECT 1 FROM usernames_retirados WHERE username = lower($1) AND hasta > now() AND user_id <> $3)`,
		username, dir, userID).Scan(&libre)
	return libre, errf("UsernameLibrePara", err)
}

// CambiarUsername pone el @usuario nuevo y reserva el anterior para la misma persona
// durante ReservaUsername, en una transacción. ErrCambioDeUsernameReciente si ya lo
// cambió dentro de ese plazo; ErrConflict si otra persona se lo ha quedado entretanto.
// Recuperar el propio @usuario reservado libera la reserva.
func (s *Store) CambiarUsername(ctx context.Context, userID, nuevo string, now time.Time) (anterior string, err error) {
	err = s.InTx(ctx, func(q pg.Querier) error {
		var cambiado *time.Time
		if err := q.QueryRow(ctx, `SELECT coalesce(username, ''), username_changed_at FROM users WHERE id = $1 FOR UPDATE`,
			userID).Scan(&anterior, &cambiado); err != nil {
			return errf("CambiarUsername.leer", err)
		}
		if cambiado != nil && now.Sub(*cambiado) < ReservaUsername {
			return ErrCambioDeUsernameReciente
		}
		// Otra persona no puede haberlo dejado reservado: lo bloquea la reserva.
		var reservado bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM usernames_retirados
			WHERE username = lower($1) AND hasta > $2 AND user_id <> $3)`, nuevo, now, userID).Scan(&reservado); err != nil {
			return errf("CambiarUsername.reserva", err)
		}
		if reservado {
			return domain.ErrConflict
		}
		if _, err := q.Exec(ctx, `UPDATE users SET username = $2, username_changed_at = $3, updated_at = now() WHERE id = $1`,
			userID, nuevo, now); err != nil {
			return errf("CambiarUsername", err)
		}
		if _, err := q.Exec(ctx, `DELETE FROM usernames_retirados WHERE username = lower($1)`, nuevo); err != nil {
			return errf("CambiarUsername.recuperar", err)
		}
		if anterior == "" {
			return nil
		}
		_, err := q.Exec(ctx, `INSERT INTO usernames_retirados (username, user_id, hasta) VALUES (lower($1), $2, $3)
			ON CONFLICT (username) DO UPDATE SET user_id = EXCLUDED.user_id, hasta = EXCLUDED.hasta`,
			anterior, userID, now.Add(ReservaUsername))
		return errf("CambiarUsername.retirar", err)
	})
	return anterior, err
}
