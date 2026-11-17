// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

type (
	PrivateStorageState = storage.LegacyMigrationState
	PrivateSourceRow    = storage.LegacySourceRow
	PrivateSourceCursor = storage.LegacySourceCursor
	PrivateSourcePage   = storage.LegacySourcePage
)

// PrivateSourceBlob is internal routing. StorageKey never appears in export JSON.
type PrivateSourceBlob struct {
	StorageKey string
	SizeBytes  int64
	Checksum   string
}

func PrivateStorageError(err error) error {
	var failure *pgconn.PgError
	if errors.As(err, &failure) && failure.Code == "PST01" {
		return domain.ErrPrivateStorageFrozen
	}
	return err
}

func privateOwner(ctx context.Context) (string, error) {
	tenant, err := tenancy.From(ctx)
	owner := OwnerFrom(ctx)
	if err != nil || !(storage.Scope{Tenant: tenant.Slug, Subject: owner, Product: "drive"}).Valid() {
		return "", domain.ErrPermission
	}
	return owner, nil
}

func privateStateTx(ctx context.Context, tx pgx.Tx, tenant, owner string) (PrivateStorageState, error) {
	value, err := pg.ReadPrivateStorageState(ctx, tx, "drive_private_storage_states", tenant, owner)
	return PrivateStorageState{Scope: storage.Scope{Tenant: tenant, Subject: owner, Product: "drive"}, Mode: value.Mode, Epoch: value.Epoch, StartedAt: value.StartedAt, Rows: value.Rows, Blobs: value.Blobs}, err
}

func (s *Store) PrivateStorageState(ctx context.Context) (PrivateStorageState, error) {
	owner, err := privateOwner(ctx)
	if err != nil {
		return PrivateStorageState{}, err
	}
	var state PrivateStorageState
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		state, e = privateStateTx(ctx, tx, tenancy.MustFrom(ctx).Slug, owner)
		return e
	})
	return state, err
}

// BeginPrivateStorageMigration waits for existing legacy writer transactions,
// blocks new ones, and captures the entire personal metadata source atomically.
// Retrying the same epoch returns that immutable snapshot. It removes nothing.
func (s *Store) BeginPrivateStorageMigration(ctx context.Context, epoch uuid.UUID) (PrivateStorageState, error) {
	owner, err := privateOwner(ctx)
	if err != nil {
		return PrivateStorageState{}, err
	}
	if epoch == uuid.Nil {
		return PrivateStorageState{}, domain.ErrPrivateStorageEpoch
	}
	tenant := tenancy.MustFrom(ctx).Slug
	var state PrivateStorageState
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := pg.EnsurePrivateStorageState(ctx, tx, "drive_private_storage_states", tenant, owner, nil); err != nil {
			return err
		}
		locked, err := pg.LockPrivateStorageState(ctx, tx, "drive_private_storage_states", tenant, owner)
		if err != nil {
			return err
		}
		mode, previous := locked.Mode, locked.Epoch
		if mode != "legacy" {
			if previous != epoch.String() || mode != "frozen" {
				return domain.ErrPrivateStorageEpoch
			}
		} else {
			// New accounts may not have opened legacy Drive. Capture its normal
			// personal root in this same transaction, before closing legacy writes.
			if _, err := ensureRootTx(ctx, tx, tenant); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE drive_private_storage_states SET mode='building',epoch=$3,started_at=clock_timestamp() WHERE tenant=$1 AND owner_sub=$2`, tenant, owner, epoch); err != nil {
				return err
			}
			metadata, err := tx.Exec(ctx, privateSourceCaptureSQL, tenant, owner, epoch)
			if err != nil {
				return fmt.Errorf("drive: capture private source: %w", err)
			}
			blobs, err := tx.Exec(ctx, privateBlobCaptureSQL, tenant, owner, epoch)
			if err != nil {
				return fmt.Errorf("drive: capture private source routing: %w", err)
			}
			if _, err := tx.Exec(ctx, `UPDATE drive_private_storage_states SET mode='frozen',source_rows=$4,source_blobs=$5 WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND mode='building'`, tenant, owner, epoch, metadata.RowsAffected(), blobs.RowsAffected()); err != nil {
				return err
			}
		}
		var e error
		state, e = privateStateTx(ctx, tx, tenant, owner)
		return e
	})
	return state, err
}

func privateEpochTx(ctx context.Context, tx pgx.Tx, tenant, owner string, epoch uuid.UUID) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drive_private_storage_states WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND mode='frozen')`, tenant, owner, epoch).Scan(&exists)
	if err != nil {
		return err
	}
	if epoch == uuid.Nil || !exists {
		return domain.ErrPrivateStorageEpoch
	}
	return nil
}

func privateSourceKind(kind string) bool {
	switch kind {
	case "folder", "file", "version", "share", "favorite", "request", "provenance":
		return true
	default:
		return false
	}
}

func (s *Store) PrivateStorageSourcePage(ctx context.Context, epoch uuid.UUID, after PrivateSourceCursor, limit int) (PrivateSourcePage, error) {
	owner, err := privateOwner(ctx)
	if err != nil {
		return PrivateSourcePage{}, err
	}
	if limit < 1 || limit > storage.MaxLegacySourceRows || (after.Kind != "" && !privateSourceKind(after.Kind)) || (after.Kind == "") != (after.ID == "") || len(after.ID) > 2048 {
		return PrivateSourcePage{}, domain.ErrPrivateStorageEpoch
	}
	tenant := tenancy.MustFrom(ctx).Slug
	result := PrivateSourcePage{Scope: storage.Scope{Tenant: tenant, Subject: owner, Product: "drive"}, Epoch: epoch.String(), Rows: []PrivateSourceRow{}}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := privateEpochTx(ctx, tx, tenant, owner, epoch); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT kind,id,payload FROM drive_private_storage_snapshot
            WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND (kind,id)>($4,$5)
            ORDER BY kind,id LIMIT $6`, tenant, owner, epoch, after.Kind, after.ID, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row PrivateSourceRow
			if err := rows.Scan(&row.Kind, &row.ID, &row.Payload); err != nil {
				return err
			}
			result.Rows = append(result.Rows, row)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(result.Rows) > limit {
			result.Rows = result.Rows[:limit]
			last := result.Rows[limit-1]
			result.Next = &PrivateSourceCursor{Kind: last.Kind, ID: last.ID}
		}
		return nil
	})
	return result, err
}

func (s *Store) PrivateStorageSourceBlob(ctx context.Context, epoch uuid.UUID, kind, id string) (PrivateSourceBlob, error) {
	owner, err := privateOwner(ctx)
	if err != nil {
		return PrivateSourceBlob{}, err
	}
	if (kind != "file" && kind != "version") || len(id) > 128 {
		return PrivateSourceBlob{}, domain.ErrNotFound
	}
	tenant := tenancy.MustFrom(ctx).Slug
	var blob PrivateSourceBlob
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := privateEpochTx(ctx, tx, tenant, owner, epoch); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT storage_key,size_bytes,checksum FROM drive_private_storage_source_blobs WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND kind=$4 AND id=$5`, tenant, owner, epoch, kind, id).Scan(&blob.StorageKey, &blob.SizeBytes, &blob.Checksum)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PrivateSourceBlob{}, domain.ErrNotFound
	}
	return blob, err
}

const privateSourceCaptureSQL = `WITH own_files AS MATERIALIZED (
 SELECT * FROM files WHERE tenant=$1 AND owner_sub=$2 AND team_drive_id IS NULL AND channel_id IS NULL
),own_folders AS MATERIALIZED (
 SELECT * FROM folders WHERE tenant=$1 AND owner_sub=$2 AND team_drive_id IS NULL AND channel_id IS NULL
),source_rows AS (
 SELECT 'file' AS kind,f.id::text AS id,to_jsonb(f)-'tenant'-'storage_key' AS payload FROM own_files f
 UNION ALL SELECT 'folder',f.id::text,to_jsonb(f)-'tenant' FROM own_folders f
 UNION ALL SELECT 'version',v.id::text,to_jsonb(v)-'tenant'-'storage_key' FROM file_versions v JOIN own_files f ON f.tenant=v.tenant AND f.id=v.file_id
 UNION ALL SELECT 'share',s.id::text,to_jsonb(s)-'tenant' FROM shares s WHERE s.tenant=$1 AND (s.file_id IN(SELECT id FROM own_files) OR s.folder_id IN(SELECT id FROM own_folders))
 UNION ALL SELECT 'favorite',v.id::text,to_jsonb(v)-'tenant' FROM favorites v WHERE v.tenant=$1 AND v.subject=$2
 UNION ALL SELECT 'request',p.recurso::text||':'||encode(convert_to(p.sujeto,'UTF8'),'hex'),to_jsonb(p)-'tenant' FROM drive_peticiones_edicion p WHERE p.tenant=$1 AND (p.sujeto=$2 OR p.recurso IN(SELECT id FROM own_files) OR p.recurso IN(SELECT id FROM own_folders))
 UNION ALL SELECT 'provenance',p.file_id::text||':'||p.version::text,to_jsonb(p)-'tenant' FROM drive_file_provenance p JOIN own_files f ON f.tenant=p.tenant AND f.id=p.file_id
)
INSERT INTO drive_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload)
 SELECT $1,$2,$3,kind,id,convert_to(payload::text,'UTF8') FROM source_rows`

const privateBlobCaptureSQL = `INSERT INTO drive_private_storage_source_blobs(tenant,owner_sub,epoch,kind,id,storage_key,size_bytes,checksum)
 SELECT $1::text,$2::text,$3::uuid,'file',f.id::text,f.storage_key,f.size_bytes,f.checksum FROM files f WHERE f.tenant=$1 AND f.owner_sub=$2 AND f.team_drive_id IS NULL AND f.channel_id IS NULL
 UNION ALL SELECT $1::text,$2::text,$3::uuid,'version',v.id::text,v.storage_key,v.size_bytes,v.checksum FROM file_versions v JOIN files f ON f.tenant=v.tenant AND f.id=v.file_id WHERE f.tenant=$1 AND f.owner_sub=$2 AND f.team_drive_id IS NULL AND f.channel_id IS NULL`
