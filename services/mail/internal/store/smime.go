// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Persistencia del material S/MIME: la autoridad de cada organización y el
// certificado de cada buzón. Las claves privadas llegan aquí YA SELLADAS por el
// servicio (AES-256-GCM con la KEK): este código nunca ve material en claro.

// Authority devuelve el certificado y la clave sellada de la autoridad.
func (s *Store) Authority(ctx context.Context, tenant string) (certPEM, keyEnc string, err error) {
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT cert_pem, key_enc FROM smime_authorities WHERE tenant = $1`, tenant).
			Scan(&certPEM, &keyEnc)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil // todavía no hay: el servicio la crea.
	}
	return certPEM, keyEnc, err
}

// SaveAuthority guarda (o rota) la autoridad de una organización.
func (s *Store) SaveAuthority(ctx context.Context, tenant, certPEM, keyEnc string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO smime_authorities (tenant, cert_pem, key_enc)
			VALUES ($1, $2, $3)
			ON CONFLICT (tenant) DO UPDATE
			SET cert_pem = EXCLUDED.cert_pem, key_enc = EXCLUDED.key_enc, updated_at = now()`,
			tenant, certPEM, keyEnc)
		return err
	})
}

// IdentityOf devuelve el material S/MIME de un buzón ("" si aún no tiene).
func (s *Store) IdentityOf(ctx context.Context, accountID uuid.UUID) (
	address, certPEM, keyEnc string, notAfter time.Time, err error,
) {
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT address, cert_pem, key_enc, not_after FROM smime_identities WHERE tenant = current_setting('app.tenant', true) AND account_id = $1`,
			accountID).Scan(&address, &certPEM, &keyEnc, &notAfter)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", time.Time{}, nil
	}
	return address, certPEM, keyEnc, notAfter, err
}

// SaveIdentity guarda (o renueva) el certificado de un buzón.
func (s *Store) SaveIdentity(
	ctx context.Context, accountID uuid.UUID, tenant, address, certPEM, keyEnc string, notAfter time.Time,
) error {
	if _, err := tenancy.From(ctx); err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO smime_identities (account_id, tenant, address, cert_pem, key_enc, not_after)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (account_id) DO UPDATE
			SET address = EXCLUDED.address, cert_pem = EXCLUDED.cert_pem,
			    key_enc = EXCLUDED.key_enc, not_after = EXCLUDED.not_after, updated_at = now()`,
			accountID, tenant, address, certPEM, keyEnc, notAfter)
		return err
	})
}
