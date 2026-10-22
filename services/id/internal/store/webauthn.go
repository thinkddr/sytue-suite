// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"math"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// --- Retos WebAuthn ---

// InsertChallenge persiste un reto WebAuthn de un solo uso.
func (s *Store) InsertChallenge(ctx context.Context, q pg.Querier, challenge []byte, userID, purpose string, expires time.Time) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO webauthn_challenges (challenge, user_id, purpose, expires_at)
		VALUES ($1, nullif($2,'')::uuid, $3, $4)`
	_, err := q.Exec(ctx, sql, challenge, userID, purpose, expires)
	return errf("InsertChallenge", err)
}

// ConsumeChallenge recupera y borra un reto válido (atómico, anti-replay).
func (s *Store) ConsumeChallenge(ctx context.Context, q pg.Querier, challenge []byte, now time.Time) (userID, purpose string, err error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `DELETE FROM webauthn_challenges
		WHERE challenge = $1 AND expires_at > $2
		RETURNING coalesce(user_id::text,''), purpose`
	err = q.QueryRow(ctx, sql, challenge, now).Scan(&userID, &purpose)
	return userID, purpose, errf("ConsumeChallenge", err)
}

// --- Credenciales WebAuthn (passkeys) ---

// InsertCredential registra una nueva passkey.
func (s *Store) InsertCredential(ctx context.Context, q pg.Querier, c domain.WebAuthnCredential) error {
	if q == nil {
		q = s.DB()
	}
	// transports es NOT NULL (text[]); una passkey puede no declarar ninguno → un
	// slice nil se enviaría como NULL y violaría la constraint (el DEFAULT '{}' solo
	// aplica si se OMITE la columna). Coaccionamos a slice vacío.
	transports := c.Transports
	if transports == nil {
		transports = []string{}
	}
	const sql = `INSERT INTO webauthn_credentials
		(user_id, cred_id, public_key, aaguid, sign_count, transports, name)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`
	_, err := q.Exec(ctx, sql, c.UserID, c.CredID, c.PublicKey, c.AAGUID,
		int64(c.SignCount), transports, defaultStr(c.Name, "Passkey"))
	return errf("InsertCredential", err)
}

// GetCredentialByID recupera una passkey por su credential id.
func (s *Store) GetCredentialByID(ctx context.Context, credID []byte) (domain.WebAuthnCredential, error) {
	const sql = `SELECT id, user_id, cred_id, public_key, aaguid, sign_count,
		transports, name, created_at, last_used_at
		FROM webauthn_credentials WHERE cred_id = $1`
	var c domain.WebAuthnCredential
	var signCount int64
	err := s.DB().QueryRow(ctx, sql, credID).Scan(&c.ID, &c.UserID, &c.CredID,
		&c.PublicKey, &c.AAGUID, &signCount, &c.Transports, &c.Name,
		&c.CreatedAt, &c.LastUsedAt)
	c.SignCount = signCountU32(signCount)
	return c, errf("GetCredentialByID", err)
}

// ListCredentialsForUser lista las passkeys de un usuario.
func (s *Store) ListCredentialsForUser(ctx context.Context, userID string) ([]domain.WebAuthnCredential, error) {
	const sql = `SELECT id, user_id, cred_id, public_key, aaguid, sign_count,
		transports, name, created_at, last_used_at
		FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at`
	rows, err := s.DB().Query(ctx, sql, userID)
	if err != nil {
		return nil, errf("ListCredentialsForUser", err)
	}
	defer rows.Close()
	var out []domain.WebAuthnCredential
	for rows.Next() {
		var c domain.WebAuthnCredential
		var signCount int64
		if err := rows.Scan(&c.ID, &c.UserID, &c.CredID, &c.PublicKey, &c.AAGUID,
			&signCount, &c.Transports, &c.Name, &c.CreatedAt, &c.LastUsedAt); err != nil {
			return nil, errf("ListCredentialsForUser.scan", err)
		}
		c.SignCount = signCountU32(signCount)
		out = append(out, c)
	}
	return out, errf("ListCredentialsForUser.iter", rows.Err())
}

// DeleteCredentialForUser borra una passkey del usuario y dice si borró algo.
// El guard user_id es anti-IDOR igual que en RevokeSessionByID: sin él, el id
// de otro serviría para dejar sin passkey a un tercero. Un id ajeno o
// inexistente devuelve (false, nil), no un error: para quien lo intenta, las
// dos situaciones son indistinguibles.
//
// Que el usuario se quede sin ninguna passkey es legítimo (puede tener
// contraseña y MFA); la comprobación de no quedarse sin ningún método de
// acceso pertenece al handler, que es quien conoce el resto de la cuenta.
func (s *Store) DeleteCredentialForUser(ctx context.Context, q pg.Querier, userID, credentialID string) (bool, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `DELETE FROM webauthn_credentials
		WHERE id = $1 AND user_id = $2`, credentialID, userID)
	if err != nil {
		return false, errf("DeleteCredentialForUser", err)
	}
	return tag.RowsAffected() > 0, nil
}

// RenameCredentialForUser cambia el nombre visible de una passkey (mismo guard
// anti-IDOR). Sirve para distinguir «MacBook» de «llave USB» cuando hay varias.
func (s *Store) RenameCredentialForUser(ctx context.Context, q pg.Querier, userID, credentialID, name string) (bool, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `UPDATE webauthn_credentials SET name = $3
		WHERE id = $1 AND user_id = $2`, credentialID, userID,
		domain.SanitizeDisplayName(name))
	if err != nil {
		return false, errf("RenameCredentialForUser", err)
	}
	return tag.RowsAffected() > 0, nil
}

// UpdateSignCount actualiza el contador de firmas (anti-clonado WebAuthn).
func (s *Store) UpdateSignCount(ctx context.Context, q pg.Querier, credID []byte, count uint32) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `UPDATE webauthn_credentials
		SET sign_count = $2, last_used_at = now() WHERE cred_id = $1`, credID, int64(count))
	return errf("UpdateSignCount", err)
}

// signCountU32 acota el sign_count persistido (BIGINT) al uint32 del protocolo
// WebAuthn; el contador se escribe siempre desde uint32, así que el clamp solo
// protege frente a datos corruptos (gosec G115).
func signCountU32(v int64) uint32 {
	if v < 0 {
		return 0
	}
	if v > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(v)
}
