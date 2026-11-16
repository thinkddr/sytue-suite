// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// CipherUsage uses the same committed rows and external routing classifier as
// quota charging. A tombstone remains charged until CompleteCipherBlobDelete.
func (s *Store) CipherUsage(ctx context.Context, product string) (storage.CipherUsage, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return storage.CipherUsage{}, err
	}
	scope := storage.Scope{Tenant: tenant.Slug, Subject: OwnerFrom(ctx), Product: product}
	if !scope.Valid() {
		return storage.CipherUsage{}, domain.ErrPermission
	}
	value := storage.CipherUsage{Version: 1, Scope: scope}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COALESCE(sum(size_bytes),0),
COALESCE(sum(size_bytes) FILTER (WHERE left(storage_key,length($4))<>$4),0),
COALESCE(sum(size_bytes) FILTER (WHERE left(storage_key,length($4))=$4),0),
COALESCE(sum(size_bytes) FILTER (WHERE deleted_at IS NOT NULL),0),count(*)
FROM cipher_blobs WHERE tenant=$1 AND owner_sub=$2 AND product=$3`, scope.Tenant, scope.Subject, scope.Product, ExternalPrefix).Scan(&value.Bytes, &value.SytueBytes, &value.ExternalBytes, &value.PendingDeleteBytes, &value.Blobs)
	})
	if err != nil {
		return storage.CipherUsage{}, err
	}
	if !value.Valid(scope) {
		return storage.CipherUsage{}, domain.ErrConflict
	}
	return value, nil
}
