// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

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
		_, err := tx.Exec(ctx, `INSERT INTO matrix_native_consents(tenant,state_hash,subject,challenge,expires_at) VALUES($1,$2,$3,$4,now()+interval '10 minutes')`, t.Slug, hash, subject, challenge)
		return err
	})
	return state, err
}

func (s *Store) PreviewMatrixNativeConsent(ctx context.Context, subject, state string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var found bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_native_consents WHERE tenant=$1 AND subject=$2 AND state_hash=$3 AND expires_at>now() AND login_secret='')`, t.Slug, subject, domain.HashToken(state)).Scan(&found); err != nil {
			return err
		}
		if !found {
			return domain.ErrForbidden
		}
		return nil
	})
}

func (s *Store) AcceptMatrixNativeConsent(ctx context.Context, subject, state, loginToken string) (string, error) {
	secret, err := s.sealSecret(loginToken)
	if err != nil {
		return "", err
	}
	ticket, hash, err := domain.NewOpaqueToken()
	if err != nil {
		return "", err
	}
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE matrix_native_consents SET login_secret=$4,ticket_hash=$5,ticket_expires_at=LEAST(expires_at,now()+interval '60 seconds') WHERE tenant=$1 AND subject=$2 AND state_hash=$3 AND expires_at>now() AND login_secret=''`, t.Slug, subject, domain.HashToken(state), secret, hash)
		if err == nil && tag.RowsAffected() != 1 {
			return domain.ErrForbidden
		}
		return err
	})
	return ticket, err
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
