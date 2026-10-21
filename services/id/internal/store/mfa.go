// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// --- Continuaciones de login MFA (F8) ---

// InsertMFAContinuation persiste una continuación de login (solo el hash del
// token). Un solo uso, corta vida y ligada a IP/UA.
func (s *Store) InsertMFAContinuation(ctx context.Context, q pg.Querier, m domain.MFAContinuation) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO mfa_continuations
		(token_hash, user_id, org_id, return_to, ip, user_agent, expires_at)
		VALUES ($1,$2,nullif($3,'')::uuid,$4,nullif($5,'')::inet,$6,$7)`
	_, err := q.Exec(ctx, sql, m.TokenHash, m.UserID, m.OrgID, m.ReturnTo,
		m.IP, m.UserAgent, m.ExpiresAt)
	return errf("InsertMFAContinuation", err)
}

// ConsumeMFAContinuation recupera y marca consumida una continuación en una sola
// operación atómica (anti-replay): solo devuelve fila si no estaba consumida ni
// expirada. El binding IP/UA lo comprueba el llamante sobre lo devuelto.
func (s *Store) ConsumeMFAContinuation(ctx context.Context, q pg.Querier, tokenHash string, now time.Time) (domain.MFAContinuation, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE mfa_continuations SET consumed_at = $2
		WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING token_hash, user_id, coalesce(org_id::text,''), return_to,
		          coalesce(host(ip),''), user_agent, expires_at`
	var m domain.MFAContinuation
	err := q.QueryRow(ctx, sql, tokenHash, now).Scan(&m.TokenHash, &m.UserID,
		&m.OrgID, &m.ReturnTo, &m.IP, &m.UserAgent, &m.ExpiresAt)
	return m, errf("ConsumeMFAContinuation", err)
}

// --- Secretos TOTP (segundo factor por app autenticadora) ---

// GetTOTPSecret devuelve el secreto TOTP confirmado del usuario (descifrado). Si
// no hay secreto confirmado, devuelve (nil, ErrNotFound).
func (s *Store) GetTOTPSecret(ctx context.Context, userID string) ([]byte, error) {
	const sql = `SELECT secret FROM totp_secrets WHERE user_id = $1 AND confirmed = true`
	var stored []byte
	if err := s.DB().QueryRow(ctx, sql, userID).Scan(&stored); err != nil {
		return nil, errf("GetTOTPSecret", err)
	}
	plain, err := s.keyCipher.Decrypt(string(stored))
	if err != nil {
		return nil, errf("GetTOTPSecret.decrypt", err)
	}
	return []byte(plain), nil
}

// GetPendingTOTPSecret devuelve el secreto TOTP NO confirmado (enrolamiento en
// curso, confirmed=false) del usuario, descifrado. (nil, ErrNotFound) si no hay
// enrolamiento pendiente. Separado de GetTOTPSecret —que solo devuelve el
// confirmado que consume el login— para no exponer al segundo factor un secreto
// a medio enrolar.
func (s *Store) GetPendingTOTPSecret(ctx context.Context, userID string) ([]byte, error) {
	const sql = `SELECT secret FROM totp_secrets WHERE user_id = $1 AND confirmed = false`
	var stored []byte
	if err := s.DB().QueryRow(ctx, sql, userID).Scan(&stored); err != nil {
		return nil, errf("GetPendingTOTPSecret", err)
	}
	plain, err := s.keyCipher.Decrypt(string(stored))
	if err != nil {
		return nil, errf("GetPendingTOTPSecret.decrypt", err)
	}
	return []byte(plain), nil
}

// UpsertTOTPSecret guarda (cifrado en reposo) el secreto TOTP de un usuario.
// confirmed indica si el usuario ya validó el enrolamiento con un código.
func (s *Store) UpsertTOTPSecret(ctx context.Context, q pg.Querier, userID string, secret []byte, confirmed bool) error {
	if q == nil {
		q = s.DB()
	}
	enc, err := s.keyCipher.Encrypt(string(secret))
	if err != nil {
		return errf("UpsertTOTPSecret.encrypt", err)
	}
	const sql = `INSERT INTO totp_secrets (user_id, secret, confirmed)
		VALUES ($1,$2,$3)
		ON CONFLICT (user_id) DO UPDATE SET secret = EXCLUDED.secret, confirmed = EXCLUDED.confirmed`
	_, err = q.Exec(ctx, sql, userID, []byte(enc), confirmed)
	return errf("UpsertTOTPSecret", err)
}

// ConfirmTOTPSecret marca como confirmado el enrolamiento TOTP pendiente del
// usuario (tras validar un código). Devuelve true si había un pendiente que
// confirmar. Ejecutar en la misma tx que SetMFAEnabled(true).
func (s *Store) ConfirmTOTPSecret(ctx context.Context, q pg.Querier, userID string) (bool, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `UPDATE totp_secrets SET confirmed = true
		WHERE user_id = $1 AND confirmed = false`, userID)
	if err != nil {
		return false, errf("ConfirmTOTPSecret", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteTOTPSecret elimina el secreto TOTP del usuario (desactivar el segundo
// factor por app). Idempotente.
func (s *Store) DeleteTOTPSecret(ctx context.Context, q pg.Querier, userID string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `DELETE FROM totp_secrets WHERE user_id = $1`, userID)
	return errf("DeleteTOTPSecret", err)
}

// --- Códigos de recuperación TOTP (segundo factor de respaldo) ---

// InsertRecoveryCodes reemplaza el lote de códigos de recuperación del usuario por
// los hashes dados (solo el hash, nunca el código en claro). Borra los previos en
// la misma operación (regenerar invalida los anteriores). Ejecutar en la tx de
// activación TOTP para que códigos y factor aparezcan a la vez.
func (s *Store) InsertRecoveryCodes(ctx context.Context, q pg.Querier, userID string, hashes []string) error {
	if q == nil {
		q = s.DB()
	}
	if _, err := q.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return errf("InsertRecoveryCodes.clear", err)
	}
	if len(hashes) == 0 {
		return nil
	}
	// unnest de un text[]: una sola sentencia para el lote (sin N+1).
	const sql = `INSERT INTO mfa_recovery_codes (user_id, code_hash)
		SELECT $1, h FROM unnest($2::text[]) AS h`
	_, err := q.Exec(ctx, sql, userID, hashes)
	return errf("InsertRecoveryCodes", err)
}

// ConsumeRecoveryCode borra (un solo uso, atómico) el código cuyo hash coincide y
// devuelve true si existía. El DELETE por PK (user_id, code_hash) es un Index Scan;
// la fila desaparece ⇒ no puede reutilizarse.
func (s *Store) ConsumeRecoveryCode(ctx context.Context, q pg.Querier, userID, codeHash string) (bool, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = $1 AND code_hash = $2`, userID, codeHash)
	if err != nil {
		return false, errf("ConsumeRecoveryCode", err)
	}
	return tag.RowsAffected() > 0, nil
}

// CountRecoveryCodes cuenta los códigos de recuperación sin usar del usuario (para
// mostrar cuántos quedan en la página de seguridad).
func (s *Store) CountRecoveryCodes(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.DB().QueryRow(ctx, `SELECT count(*) FROM mfa_recovery_codes WHERE user_id = $1`, userID).Scan(&n)
	return n, errf("CountRecoveryCodes", err)
}

// DeleteRecoveryCodes borra todos los códigos del usuario (al desactivar el TOTP).
// Idempotente. Ejecutar en la misma tx que DeleteTOTPSecret.
func (s *Store) DeleteRecoveryCodes(ctx context.Context, q pg.Querier, userID string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = $1`, userID)
	return errf("DeleteRecoveryCodes", err)
}

// ResetUserMFA borra TODO el segundo factor del usuario (TOTP + passkeys WebAuthn +
// códigos de recuperación) y baja mfa_enabled, para que re-enrole desde cero. Es la
// contrapartida admin del desenrolamiento self-service.
//
// Aislamiento por tenant (anti-IDOR): comprueba EN LA MISMA operación que el
// usuario es miembro de orgID; si no, devuelve ErrNotFound sin tocar nada (no se
// resetea el MFA de un usuario de otro tenant). El handler ya lo verifica, pero el
// gate vive también aquí para que ninguna ruta pueda saltárselo. Idempotente:
// borrar cuando no hay MFA no es error. Ejecutar dentro de una tx (varias tablas).
func (s *Store) ResetUserMFA(ctx context.Context, q pg.Querier, orgID, userID string) error {
	if q == nil {
		q = s.DB()
	}
	var one int
	if err := q.QueryRow(ctx,
		`SELECT 1 FROM memberships WHERE org_id=$1 AND user_id=$2`, orgID, userID).Scan(&one); err != nil {
		return errf("ResetUserMFA.membership", err) // ErrNoRows → ErrNotFound (cross-tenant)
	}
	if _, err := q.Exec(ctx, `DELETE FROM totp_secrets WHERE user_id = $1`, userID); err != nil {
		return errf("ResetUserMFA.totp", err)
	}
	if _, err := q.Exec(ctx, `DELETE FROM webauthn_credentials WHERE user_id = $1`, userID); err != nil {
		return errf("ResetUserMFA.webauthn", err)
	}
	if _, err := q.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return errf("ResetUserMFA.recovery", err)
	}
	// Sin ningún factor, mfa_enabled=true dejaría al usuario sin poder superar el
	// step-up del login (lockout); se baja siempre, como disableTOTP.
	_, err := q.Exec(ctx, `UPDATE users SET mfa_enabled=false WHERE id=$1`, userID)
	return errf("ResetUserMFA.flag", err)
}
