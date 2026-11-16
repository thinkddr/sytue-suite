// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"

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
	return s.CreateMatrixNativeConsentForIdentity(ctx, subject, challenge, product, storage.LegacyIdentity)
}

func (s *Store) CreateMatrixNativeConsentForIdentity(ctx context.Context, subject, challenge, product string, version storage.IdentityVersion) (string, error) {
	return s.CreateMatrixNativeConsentForConsumer(ctx, subject, challenge, product, "", version)
}

func (s *Store) CreateMatrixNativeConsentForConsumer(ctx context.Context, subject, challenge, product, consumer string, version storage.IdentityVersion) (string, error) {
	if storage.NativeReturnApp(product, consumer) == "" || !version.Valid() {
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
	err = s.withMatrixConsentIdentity(ctx, subject, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM matrix_native_consents WHERE tenant=$1 AND expires_at<now()`, t.Slug); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO matrix_native_consents(tenant,state_hash,subject,challenge,product,identity_version,native_consumer,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,now()+interval '10 minutes')`, t.Slug, hash, subject, challenge, product, version, consumer)
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
	destination, err := s.PreviewMatrixNativeDestination(ctx, subject, state)
	return destination.Product, err
}

type MatrixNativeDestination struct {
	Purpose          string
	ResourceApproved bool
	NativeConsumer   string
	Product          string
	IdentityVersion  storage.IdentityVersion
}

// MatrixNativeConsentTenant resolves only the immutable routing context of an
// authenticated person's opaque grant. The service must prove live membership
// and repeat the atomic operation under the destination's lifecycle locks.
// Legacy grants retain their original same-tenant behavior.
func (s *Store) MatrixNativeConsentTenant(ctx context.Context, subject, state string) (tenancy.Tenant, error) {
	var tenant string
	var version storage.IdentityVersion
	var purpose string
	err := s.withMatrixNativeConsumerContract(ctx, func(tx pgx.Tx, _ string) error {
		return tx.QueryRow(ctx, `SELECT tenant,identity_version,purpose FROM matrix_native_consents WHERE subject=$1 AND state_hash=$2 AND expires_at>now()`, subject, domain.HashToken(state)).Scan(&tenant, &version, &purpose)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return tenancy.Tenant{}, domain.ErrForbidden
	}
	if err != nil {
		return tenancy.Tenant{}, err
	}
	if !version.Valid() || (version != storage.IsolatedIdentity && purpose == "" && tenant != tenancy.MustFrom(ctx).Slug) {
		return tenancy.Tenant{}, domain.ErrForbidden
	}
	return tenancy.New(tenant)
}

func (s *Store) PreviewMatrixNativeDestination(ctx context.Context, subject, state string) (MatrixNativeDestination, error) {
	t := tenancy.MustFrom(ctx)
	var destination MatrixNativeDestination
	err := s.withMatrixConsentIdentity(ctx, subject, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT product,identity_version,native_consumer,purpose,resource_approved_at IS NOT NULL FROM matrix_native_consents WHERE tenant=$1 AND subject=$2 AND state_hash=$3 AND expires_at>now() AND login_secret=''`, t.Slug, subject, domain.HashToken(state)).Scan(&destination.Product, &destination.IdentityVersion, &destination.NativeConsumer, &destination.Purpose, &destination.ResourceApproved)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return MatrixNativeDestination{}, domain.ErrForbidden
	}
	return destination, err
}

// AcceptMatrixNativeConsent preserves the existing Rooms-only store contract.
func (s *Store) AcceptMatrixNativeConsent(ctx context.Context, subject, state, loginToken string) (string, error) {
	ticket, _, err := s.AcceptMatrixNativeConsentWithProduct(ctx, subject, state, loginToken)
	return ticket, err
}

// AcceptMatrixNativeConsentWithProduct binds the one-use ticket and reads the
// original destination atomically; concurrent callbacks cannot replace either.
func (s *Store) AcceptMatrixNativeConsentWithProduct(ctx context.Context, subject, state, loginToken string) (string, string, error) {
	ticket, destination, err := s.AcceptMatrixNativeConsentWithDestination(ctx, subject, state, loginToken)
	return ticket, destination.Product, err
}

func (s *Store) AcceptMatrixNativeConsentWithDestination(ctx context.Context, subject, state, loginToken string) (string, MatrixNativeDestination, error) {
	secret, err := s.sealSecret(loginToken)
	if err != nil {
		return "", MatrixNativeDestination{}, err
	}
	ticket, hash, err := domain.NewOpaqueToken()
	if err != nil {
		return "", MatrixNativeDestination{}, err
	}
	t := tenancy.MustFrom(ctx)
	var destination MatrixNativeDestination
	err = s.withMatrixConsentIdentity(ctx, subject, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE matrix_native_consents SET login_secret=$4,ticket_hash=$5,ticket_expires_at=LEAST(expires_at,now()+interval '60 seconds') WHERE tenant=$1 AND subject=$2 AND state_hash=$3 AND expires_at>now() AND login_secret='' AND (purpose='' OR resource_approved_at IS NOT NULL) RETURNING product,identity_version,native_consumer,purpose,resource_approved_at IS NOT NULL`, t.Slug, subject, domain.HashToken(state), secret, hash).Scan(&destination.Product, &destination.IdentityVersion, &destination.NativeConsumer, &destination.Purpose, &destination.ResourceApproved)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrForbidden
		}
		return err
	})
	return ticket, destination, err
}

func (s *Store) FinishMatrixNativeConsent(ctx context.Context, subject, state, ticket, challenge string) (string, error) {
	return s.FinishMatrixNativeConsentForIdentity(ctx, subject, state, ticket, challenge, "", storage.LegacyIdentity)
}

func (s *Store) FinishMatrixNativeConsentForIdentity(ctx context.Context, subject, state, ticket, challenge, product string, version storage.IdentityVersion) (string, error) {
	return s.FinishMatrixNativeConsentForConsumer(ctx, subject, state, ticket, challenge, product, "", version)
}

func (s *Store) FinishMatrixNativeConsentForConsumer(ctx context.Context, subject, state, ticket, challenge, product, consumer string, version storage.IdentityVersion) (string, error) {
	if consumer != "" && storage.NativeReturnApp(product, consumer) == "" {
		return "", domain.ErrInvalidInput
	}
	if !version.Valid() || (product != "" && !storage.Product(product)) || (version == storage.IsolatedIdentity && product == "") {
		return "", domain.ErrInvalidInput
	}
	var token string
	err := s.withMatrixConsentIdentity(ctx, subject, func(tx pgx.Tx) error {
		value, err := s.consumeMatrixNativeTicketTx(ctx, tx, subject, state, ticket, challenge, product, consumer, version, "")
		token = value.token
		return err
	})
	return token, err
}

// Consent creation and exchange serialize with durable identity/tenant denial.
// A grant cannot arrive after the revoker's DELETE and survive reactivation.
func (s *Store) withMatrixConsentIdentity(ctx context.Context, subject string, fn func(pgx.Tx) error) error {
	return s.withMatrixNativeConsumerContract(ctx, func(tx pgx.Tx, _ string) error {
		return s.WithMatrixIdentity(matrixContext(ctx, tx), subject, func(context.Context) error { return fn(tx) })
	})
}

// The new broker opts into consumer rows explicitly. The restrictive RLS fence
// hides those rows from old rolling-deploy binaries, even under maintenance.
func (s *Store) withMatrixNativeConsumerContract(ctx context.Context, fn func(pgx.Tx, string) error) error {
	return s.enMantenimientoDe(ctx, func(tx pgx.Tx, tenant string) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_native_consumer_contract','1',true),set_config('app.matrix_resource_session_contract','1',true)`); err != nil {
			return err
		}
		return fn(tx, tenant)
	})
}
