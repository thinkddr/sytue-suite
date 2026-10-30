// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

func (s *Store) CipherBlob(ctx context.Context, product, id string) (domain.CipherBlob, error) {
	var blob domain.CipherBlob
	if !storage.Product(product) || !storage.BlobID(id) || OwnerFrom(ctx) == "" {
		return blob, domain.ErrNotFound
	}
	t := tenancy.MustFrom(ctx)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id,product,size_bytes,sha256,storage_key,owner_sub FROM cipher_blobs WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND id=$4 AND deleted_at IS NULL`, t.Slug, OwnerFrom(ctx), product, id).Scan(&blob.ID, &blob.Product, &blob.SizeBytes, &blob.SHA256, &blob.StorageKey, &blob.OwnerSub)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return blob, domain.ErrNotFound
	}
	return blob, err
}

func (s *Store) CreateCipherBlob(ctx context.Context, blob domain.CipherBlob) (domain.CipherBlob, error) {
	if !blob.Valid(blob.Product, blob.ID) || blob.StorageKey == "" || blob.OwnerSub == "" || blob.OwnerSub != OwnerFrom(ctx) {
		return domain.CipherBlob{}, domain.ErrPermission
	}
	t := tenancy.MustFrom(ctx)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO cipher_blobs(tenant,owner_sub,product,id,storage_key,size_bytes,sha256) VALUES($1,$2,$3,$4,$5,$6,$7)`, t.Slug, blob.OwnerSub, blob.Product, blob.ID, blob.StorageKey, blob.SizeBytes, blob.SHA256)
		if err != nil {
			return err
		}
		var reserved int64
		var charged bool
		if err := tx.QueryRow(ctx, `DELETE FROM subidas_pendientes WHERE tenant=$1 AND storage_key=$2 AND caduca_en>now() AND NOT finalizada RETURNING size_bytes,con_cuota`, t.Slug, blob.StorageKey).Scan(&reserved, &charged); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrConflict
			}
			return err
		}
		if reserved != blob.SizeBytes || charged == EsExterno(blob.StorageKey) {
			return domain.ErrPermission
		}
		return reserveQuota(ctx, tx, t.Slug, bytesEnCuota(blob.StorageKey, blob.SizeBytes))
	})
	if uniqueViolation(err) {
		return domain.CipherBlob{}, domain.ErrConflict
	}
	return blob, err
}

func (s *Store) DeleteCipherBlob(ctx context.Context, product, id string) (domain.CipherBlob, error) {
	var blob domain.CipherBlob
	if !storage.Product(product) || !storage.BlobID(id) || OwnerFrom(ctx) == "" {
		return blob, domain.ErrNotFound
	}
	t := tenancy.MustFrom(ctx)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id,product,size_bytes,sha256,storage_key,owner_sub FROM cipher_blobs WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND id=$4 FOR UPDATE`, t.Slug, OwnerFrom(ctx), product, id).Scan(&blob.ID, &blob.Product, &blob.SizeBytes, &blob.SHA256, &blob.StorageKey, &blob.OwnerSub); err != nil {
			return err
		}
		held, err := subjectUnderHoldTx(ctx, tx, t.Slug, blob.OwnerSub)
		if err != nil {
			return err
		}
		if held {
			return domain.ErrLegalHold
		}
		_, err = tx.Exec(ctx, `UPDATE cipher_blobs SET deleted_at=COALESCE(deleted_at,now()) WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND id=$4`, t.Slug, blob.OwnerSub, product, id)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return blob, domain.ErrNotFound
	}
	return blob, err
}

// CompleteCipherBlobDelete releases quota only after the provider confirmed the
// physical delete. The tombstone retains its routing reference across retries.
func (s *Store) CompleteCipherBlobDelete(ctx context.Context, blob domain.CipherBlob) error {
	if blob.OwnerSub == "" || blob.OwnerSub != OwnerFrom(ctx) {
		return domain.ErrPermission
	}
	t := tenancy.MustFrom(ctx)
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var actual int64
		err := tx.QueryRow(ctx, `DELETE FROM cipher_blobs WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND id=$4 AND storage_key=$5 AND deleted_at IS NOT NULL RETURNING size_bytes`, t.Slug, blob.OwnerSub, blob.Product, blob.ID, blob.StorageKey).Scan(&actual)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		return addUsage(ctx, tx, t.Slug, -bytesEnCuota(blob.StorageKey, actual))
	})
}
