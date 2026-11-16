// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

type resourceLifecycleVersion struct{ Account, Tenant int64 }

func nativeResourceClient(clientID string) bool {
	return clientID == "sytue-desktop" || clientID == "sytue-mobile"
}

// resourceSessionRowsTx follows the existing lifecycle lock order. The only
// maintenance reads are account/tenant versions; registry queries run after
// maintenance is OFF with the normal tenant/product/subject RLS policy.
func (s *Store) resourceSessionRowsTx(ctx context.Context, tx pgx.Tx, scope storage.Scope) (resourceLifecycleVersion, error) {
	var out resourceLifecycleVersion
	tenant, err := tenancy.From(ctx)
	if err != nil || !scope.Valid() || tenant.Slug != scope.Tenant {
		return out, domain.ErrForbidden
	}
	err = tx.QueryRow(ctx, `SELECT i.version,t.version FROM matrix_identity_access i JOIN matrix_tenant_access t ON t.subject=i.subject AND t.tenant=$1 WHERE i.subject=$2 AND i.status='active' AND t.status='active'`, scope.Tenant, scope.Subject).Scan(&out.Account, &out.Tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrForbidden
	}
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, `SELECT set_config('app.maintenance','off',true),set_config('app.resource_session_subject',$1,true),set_config('app.resource_session_product',$2,true),set_config('app.matrix_resource_session_contract','1',true)`, scope.Subject, scope.Product)
	return out, err
}

func (s *Store) withResourceSessionRows(ctx context.Context, scope storage.Scope, fn func(context.Context, pgx.Tx, resourceLifecycleVersion) error) error {
	return s.withMatrixConsentIdentity(ctx, scope.Subject, func(tx pgx.Tx) error {
		versions, err := s.resourceSessionRowsTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		return fn(ctx, tx, versions)
	})
}

func (s *Store) CreateMatrixResourceNativeConsent(ctx context.Context, subject, challenge, product, clientID string) (string, error) {
	if !storage.Product(product) || !nativeResourceClient(clientID) || len(challenge) != 43 {
		return "", domain.ErrInvalidInput
	}
	tenant := tenancy.MustFrom(ctx)
	if tenant.Personal() && (product == "erp" || product == "bases") {
		return "", domain.ErrForbidden
	}
	if !s.cipher.Enabled() {
		return "", ErrNoCipher
	}
	state, hash, err := domain.NewOpaqueToken()
	if err != nil {
		return "", err
	}
	scope := storage.Scope{Tenant: tenant.Slug, Subject: subject, Product: product}
	err = s.withResourceSessionRows(ctx, scope, func(ctx context.Context, tx pgx.Tx, _ resourceLifecycleVersion) error {
		_, err := tx.Exec(ctx, `INSERT INTO matrix_native_consents(tenant,state_hash,subject,challenge,product,identity_version,native_consumer,purpose,resource_client_id,expires_at) VALUES($1,$2,$3,$4,$5,1,'','resources:'||$5,$6,now()+interval '10 minutes')`, scope.Tenant, hash, subject, challenge, product, clientID)
		return err
	})
	return state, err
}

func (s *Store) ApproveMatrixResourceNativeConsent(ctx context.Context, subject, state string) error {
	destination, err := s.PreviewMatrixNativeDestination(ctx, subject, state)
	if err != nil {
		return err
	}
	if destination.Purpose != "resources:"+destination.Product {
		return domain.ErrForbidden
	}
	scope := storage.Scope{Tenant: tenancy.MustFrom(ctx).Slug, Subject: subject, Product: destination.Product}
	return s.withResourceSessionRows(ctx, scope, func(ctx context.Context, tx pgx.Tx, _ resourceLifecycleVersion) error {
		command, err := tx.Exec(ctx, `UPDATE matrix_native_consents SET resource_approved_at=COALESCE(resource_approved_at,now()) WHERE tenant=$1 AND subject=$2 AND state_hash=$3 AND purpose='resources:'||product AND expires_at>now() AND login_secret=''`, scope.Tenant, subject, domain.HashToken(state))
		if err == nil && command.RowsAffected() != 1 {
			return domain.ErrForbidden
		}
		return err
	})
}

type MatrixResourceNativeGrant struct {
	LoginToken string `json:"login_token,omitempty"`
	Product    string `json:"product"`
	Namespace  string `json:"namespace"`
	Tenant     string `json:"tenant"`
	ConsentID  string `json:"consent_id"`
	Grant      string `json:"grant"`
}

// FinishMatrixResourceNativeConsent adopts only the ORIGINAL approved purpose
// and client. The one-use key-possession grant is created in the same transaction
// as consuming PKCE; no retry can exchange a legacy/Storage ticket into Docs.
func (s *Store) FinishMatrixResourceNativeConsent(ctx context.Context, subject, state, ticket, challenge, clientID string) (MatrixResourceNativeGrant, error) {
	var out MatrixResourceNativeGrant
	if !nativeResourceClient(clientID) {
		return out, domain.ErrForbidden
	}
	raw, hash, err := domain.NewOpaqueToken()
	if err != nil {
		return out, err
	}
	grantID := uuid.NewString()
	err = s.withMatrixConsentIdentity(ctx, subject, func(tx pgx.Tx) error {
		original, err := s.consumeMatrixNativeTicketTx(ctx, tx, subject, state, ticket, challenge, "", "", storage.LegacyIdentity, clientID)
		if err != nil {
			return err
		}
		if original.purpose != "resources:"+original.product || original.approvedAt == nil || original.clientID != clientID {
			return domain.ErrForbidden
		}
		scope := storage.Scope{Tenant: tenancy.MustFrom(ctx).Slug, Subject: subject, Product: original.product}
		if tenancy.MustFrom(ctx).Personal() && (scope.Product == "erp" || scope.Product == "bases") {
			return domain.ErrForbidden
		}
		versions, err := s.resourceSessionRowsTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		err = insertResourceConsentTx(ctx, tx, scope, versions, grantID, "native", clientID, hash, domain.HashToken(state), *original.approvedAt)
		if err != nil {
			return err
		}
		out = MatrixResourceNativeGrant{LoginToken: original.token, Product: scope.Product, Namespace: original.purpose, Tenant: scope.Tenant, ConsentID: grantID, Grant: raw}
		return nil
	})
	if err != nil {
		return MatrixResourceNativeGrant{}, err
	}
	return out, nil
}
