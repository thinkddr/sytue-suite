// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// SigningKeyRow es una clave de firma JWKS persistida.
type SigningKeyRow struct {
	KID        string
	Algorithm  string
	PublicJWK  json.RawMessage
	PrivatePEM string
	Status     string
}

// EncryptPrivatePEM cifra el PEM de la clave privada para almacenarlo en reposo
// (F9). Con cifrado deshabilitado (dev) devuelve el valor sin cambios.
func (s *Store) EncryptPrivatePEM(pem string) (string, error) {
	return s.keyCipher.Encrypt(pem)
}

// InsertSigningKey persiste una nueva clave de firma. private_pem se cifra en
// reposo (AES-256-GCM) si hay clave de cifrado configurada.
func (s *Store) InsertSigningKey(ctx context.Context, q pg.Querier, k SigningKeyRow) error {
	if q == nil {
		q = s.DB()
	}
	enc, err := s.keyCipher.Encrypt(k.PrivatePEM)
	if err != nil {
		return errf("InsertSigningKey.encrypt", err)
	}
	const sql = `INSERT INTO signing_keys (kid, algorithm, public_jwk, private_pem, status)
		VALUES ($1,$2,$3,$4,$5)`
	_, err = q.Exec(ctx, sql, k.KID, k.Algorithm, []byte(k.PublicJWK), enc, k.Status)
	return errf("InsertSigningKey", err)
}

// ListSigningKeys devuelve las claves no retiradas (active+next) para el JWKS
// y la firma. La clave 'active' es la usada para firmar nuevos tokens.
func (s *Store) ListSigningKeys(ctx context.Context) ([]SigningKeyRow, error) {
	const sql = `SELECT kid, algorithm, public_jwk, private_pem, status
		FROM signing_keys WHERE status <> 'retired'
		ORDER BY (status='active') DESC, created_at DESC`
	rows, err := s.DB().Query(ctx, sql)
	if err != nil {
		return nil, errf("ListSigningKeys", err)
	}
	defer rows.Close()
	var out []SigningKeyRow
	for rows.Next() {
		var k SigningKeyRow
		var pub []byte
		var stored string
		if err := rows.Scan(&k.KID, &k.Algorithm, &pub, &stored, &k.Status); err != nil {
			return nil, errf("ListSigningKeys.scan", err)
		}
		// Descifrar private_pem en reposo (o pasar el PEM en claro heredado).
		pem, err := s.keyCipher.Decrypt(stored)
		if err != nil {
			return nil, errf("ListSigningKeys.decrypt", err)
		}
		k.PrivatePEM = pem
		k.PublicJWK = json.RawMessage(pub)
		out = append(out, k)
	}
	return out, errf("ListSigningKeys.iter", rows.Err())
}

// CountSigningKeys cuenta las claves activas (para el bootstrap inicial).
func (s *Store) CountSigningKeys(ctx context.Context) (int, error) {
	var n int
	err := s.DB().QueryRow(ctx, `SELECT count(*) FROM signing_keys WHERE status='active'`).Scan(&n)
	return n, errf("CountSigningKeys", err)
}

// RetireStaleKeys retira ('retired') las claves degradadas a 'next' cuya ventana
// de gracia (demoted_at + retireAfter) ya venció, y devuelve cuántas retiró. La
// ventana debe ser >= vida del access token para no invalidar tokens en vuelo
// (F10). Las claves retiradas dejan de publicarse en el JWKS (ListSigningKeys las
// excluye) y ya no pueden verificar firmas.
func (s *Store) RetireStaleKeys(ctx context.Context, now time.Time, retireAfter time.Duration) (int64, error) {
	const sql = `UPDATE signing_keys
		SET status='retired', retired_at=$1
		WHERE status='next' AND demoted_at IS NOT NULL AND demoted_at < $2`
	tag, err := s.DB().Exec(ctx, sql, now, now.Add(-retireAfter))
	if err != nil {
		return 0, errf("RetireStaleKeys", err)
	}
	return tag.RowsAffected(), nil
}
