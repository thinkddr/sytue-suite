// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	suiteconfig "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func insertResourceConsentTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, versions resourceLifecycleVersion, id, source, client, grantHash, sourceHash string, approved time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO matrix_resource_consents(id,tenant,subject,product,namespace,account_version,tenant_version,source,client_id,grant_hash,source_hash,approved_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now()+interval '10 minutes')`, id, scope.Tenant, scope.Subject, scope.Product, "resources:"+scope.Product, versions.Account, versions.Tenant, source, client, grantHash, sourceHash, approved)
	return err
}

// CreateMatrixResourceWebConsent is called only after the service verifies the
// CURRENT web client for this product. A native shared audience must use the
// visible browser/PKCE handoff instead and cannot mint this grant directly.
func (s *Store) CreateMatrixResourceWebConsent(ctx context.Context, subject, product, clientID string) (MatrixResourceNativeGrant, error) {
	var out MatrixResourceNativeGrant
	if !storage.Product(product) || clientID == "" || clientID != suiteconfig.FirstPartyProductClientID(product) || nativeResourceClient(clientID) {
		return out, domain.ErrForbidden
	}
	tenant := tenancy.MustFrom(ctx)
	if tenant.Personal() && (product == "erp" || product == "bases") {
		return out, domain.ErrForbidden
	}
	raw, hash, err := domain.NewOpaqueToken()
	if err != nil {
		return out, err
	}
	_, sourceHash, err := domain.NewOpaqueToken()
	if err != nil {
		return out, err
	}
	id := uuid.NewString()
	scope := storage.Scope{Tenant: tenant.Slug, Subject: subject, Product: product}
	err = s.withResourceSessionRows(ctx, scope, func(ctx context.Context, tx pgx.Tx, versions resourceLifecycleVersion) error {
		return insertResourceConsentTx(ctx, tx, scope, versions, id, "web", clientID, hash, sourceHash, time.Now())
	})
	if err != nil {
		return out, err
	}
	return MatrixResourceNativeGrant{Product: product, Namespace: "resources:" + product, Tenant: scope.Tenant, ConsentID: id, Grant: raw}, nil
}
