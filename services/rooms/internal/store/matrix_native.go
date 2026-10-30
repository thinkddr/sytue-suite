// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// MatrixActorAllowed checks durable account and tenant denials in addition to
// ID's current membership proof. It never clears or resumes lifecycle state.
func (s *Store) MatrixActorAllowed(ctx context.Context, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		var denied bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_identity_access WHERE subject=$2 AND status<>'active') OR EXISTS(SELECT 1 FROM matrix_tenant_access WHERE tenant=$1 AND subject=$2 AND status<>'active')`, t.Slug, subject).Scan(&denied); err != nil {
			return err
		}
		if denied {
			return domain.ErrForbidden
		}
		return nil
	})
}

func (s *Store) CreateMatrixNativeConsent(ctx context.Context, subject, challenge string) (string, error) {
	return s.CreateMatrixNativeConsentFor(ctx, subject, challenge, "rooms")
}

func (s *Store) CreateMatrixNativeConsentFor(ctx context.Context, subject, challenge, product string) (string, error) {
	if storage.NativeApp(product) == "" {
		return "", domain.ErrInvalidInput
	}

	if !s.cipher.Enabled() {
		return "", ErrNoCipher
	}
	state, hash, err := domain.NewOpaqueToken()
	if err != nil {
		return "", err
	}
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM matrix_native_consents WHERE tenant=$1 AND expires_at<now()`, t.Slug); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO matrix_native_consents(tenant,state_hash,subject,challenge,product,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '10 minutes')`, t.Slug, hash, subject, challenge, product)
		return err
	})
	return state, err
}

func (s *Store) PreviewMatrixNativeConsent(ctx context.Context, subject, state string) error {
	_, err := s.PreviewMatrixNativeProduct(ctx, subject, state)
	return err
}

// PreviewMatrixNativeProduct checks the unconsumed grant and returns its fixed
// destination in one transaction. Browser queries never choose this product.
func (s *Store) PreviewMatrixNativeProduct(ctx context.Context, subject, state string) (string, error) {
	t := tenancy.MustFrom(ctx)
	var product string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT product FROM matrix_native_consents WHERE tenant=$1 AND subject=$2 AND state_hash=$3 AND expires_at>now() AND login_secret=''`, t.Slug, subject, domain.HashToken(state)).Scan(&product)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrForbidden
	}
	return product, err
}

// AcceptMatrixNativeConsent preserves the existing Rooms-only store contract.
func (s *Store) AcceptMatrixNativeConsent(ctx context.Context, subject, state, loginToken string) (string, error) {
	ticket, _, err := s.AcceptMatrixNativeConsentWithProduct(ctx, subject, state, loginToken)
	return ticket, err
}

// AcceptMatrixNativeConsentWithProduct binds the one-use ticket and reads the
// original destination atomically; concurrent callbacks cannot replace either.
func (s *Store) AcceptMatrixNativeConsentWithProduct(ctx context.Context, subject, state, loginToken string) (string, string, error) {
	secret, err := s.sealSecret(loginToken)
	if err != nil {
		return "", "", err
	}
	ticket, hash, err := domain.NewOpaqueToken()
	if err != nil {
		return "", "", err
	}
	t := tenancy.MustFrom(ctx)
	var product string
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE matrix_native_consents SET login_secret=$4,ticket_hash=$5,ticket_expires_at=LEAST(expires_at,now()+interval '60 seconds') WHERE tenant=$1 AND subject=$2 AND state_hash=$3 AND expires_at>now() AND login_secret='' RETURNING product`, t.Slug, subject, domain.HashToken(state), secret, hash).Scan(&product)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrForbidden
		}
		return err
	})
	return ticket, product, err
}

func (s *Store) FinishMatrixNativeConsent(ctx context.Context, subject, state, ticket, challenge string) (string, error) {
	t := tenancy.MustFrom(ctx)
	var token string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var secret string
		var expires time.Time
		err := tx.QueryRow(ctx, `SELECT login_secret,ticket_expires_at FROM matrix_native_consents WHERE tenant=$1 AND subject=$2 AND state_hash=$3 AND ticket_hash=$4 AND challenge=$5 AND expires_at>now() AND ticket_expires_at>now() AND login_secret<>'' FOR UPDATE`, t.Slug, subject, domain.HashToken(state), domain.HashToken(ticket), challenge).Scan(&secret, &expires)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrForbidden
		}
		if err != nil {
			return err
		}
		if !s.cipher.Enabled() {
			return ErrNoCipher
		}
		token, err = s.openSecret(secret)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM matrix_native_consents WHERE tenant=$1 AND state_hash=$2`, t.Slug, domain.HashToken(state))
		return err
	})
	return token, err
}
