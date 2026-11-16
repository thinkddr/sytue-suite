// SPDX-License-Identifier: Apache-2.0
package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func preflightInfo(scope storage.Scope, epoch string, row photoPreflight) model.SourcePreflightInfo {
	return model.SourcePreflightInfo{Scope: scope, Epoch: epoch, CAS: row.ticket, SHA256: row.inventory, Rows: row.rows, Blobs: row.blobs, Originals: row.originals}
}

// StartPreflight enumerates all SQL rows and historical S3 paths under the
// existing owner fence, without loading the library into the service process.
func (p *PrivateSource) StartPreflight(ctx context.Context, scope storage.Scope, epoch string) (model.SourcePreflightInfo, error) {
	if !storage.MigrationEpoch(epoch) {
		return model.SourcePreflightInfo{}, ErrPrivateSource
	}
	var result model.SourcePreflightInfo
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		if err := lockPhotoLegacySource(ctx, tx, scope); err != nil {
			return err
		}
		capture, err := capturePhotoSourceBounded(ctx, tx, p.objects, scope, epoch)
		if err != nil {
			return err
		}
		row, err := retainPhotoPreflight(ctx, tx, scope, epoch, capture)
		if err != nil {
			return err
		}
		result = preflightInfo(scope, epoch, row)
		return result.Validate(scope, epoch)
	})
	return result, err
}

func retainPhotoPreflight(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch string, capture photoCaptureSummary) (photoPreflight, error) {
	row, err := readPhotoPreflight(ctx, tx, scope, epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		row = photoPreflight{ticket: uuid.NewString(), inventory: capture.SHA256, rows: capture.Rows, blobs: capture.Blobs, originals: capture.Originals}
		_, err = tx.Exec(ctx, `INSERT INTO photos_private_storage_preflights(tenant,owner_sub,epoch,ticket,inventory_sha256,backend_rows,backend_blobs,original_blobs)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, scope.Tenant, scope.Subject, epoch, row.ticket, row.inventory, row.rows, row.blobs, row.originals)
	}
	if err != nil {
		return row, err
	}
	if !capture.matches(row) {
		return row, ErrPrivateSource
	}
	_, err = tx.Exec(ctx, `INSERT INTO photos_private_storage_preflight_inventory(tenant,owner_sub,epoch,kind,id,size_bytes,sha256,storage_key,media_payload)
 SELECT $1,$2,$3,kind,id,size_bytes,sha256,storage_key,CASE WHEN kind='media' THEN payload END FROM pg_temp.sytue_photos_capture
 ON CONFLICT(tenant,owner_sub,epoch,kind,id) DO NOTHING`, scope.Tenant, scope.Subject, epoch)
	if err != nil {
		return row, err
	}
	var same bool
	err = tx.QueryRow(ctx, `SELECT NOT EXISTS(
 (SELECT kind,id,size_bytes,sha256,storage_key,CASE WHEN kind='media' THEN payload END FROM pg_temp.sytue_photos_capture EXCEPT
  SELECT kind,id,size_bytes,sha256,storage_key,media_payload FROM photos_private_storage_preflight_inventory WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3)
 UNION ALL
 (SELECT kind,id,size_bytes,sha256,storage_key,media_payload FROM photos_private_storage_preflight_inventory WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 EXCEPT
  SELECT kind,id,size_bytes,sha256,storage_key,CASE WHEN kind='media' THEN payload END FROM pg_temp.sytue_photos_capture))`, scope.Tenant, scope.Subject, epoch).Scan(&same)
	if err != nil {
		return row, err
	}
	if !same {
		return row, ErrPrivateSource
	}
	return row, nil
}

func verifyPhotoPreflight(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch, ticket string) error {
	if !storage.MigrationEpoch(epoch) || !storage.MigrationEpoch(ticket) {
		return ErrPrivateSource
	}
	row, err := readPhotoPreflight(ctx, tx, scope, epoch)
	if err != nil {
		return err
	}
	if row.ticket != ticket {
		return ErrPrivateSource
	}
	return preflightInfo(scope, epoch, row).Validate(scope, epoch)
}

func (p *PrivateSource) PreflightPage(ctx context.Context, scope storage.Scope, epoch, ticket string, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	if limit < 1 || limit > storage.MaxLegacySourceRows || after != (storage.LegacySourceCursor{}) && (!storage.SourceCursorValid(after) || !sourceKind(after.Kind)) {
		return storage.StreamSourcePage{}, ErrPrivateSource
	}
	var result storage.StreamSourcePage
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		if err := verifyPhotoPreflight(ctx, tx, scope, epoch, ticket); err != nil {
			return err
		}
		if after != (storage.LegacySourceCursor{}) {
			var present bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM photos_private_storage_preflight_inventory WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND kind=$4 AND id=$5)`, scope.Tenant, scope.Subject, epoch, after.Kind, after.ID).Scan(&present); err != nil {
				return err
			}
			if !present {
				return ErrPrivateSource
			}
		}
		rows, err := tx.Query(ctx, `SELECT kind,id,size_bytes,sha256 FROM photos_private_storage_preflight_inventory WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND(kind,id)>($4,$5) ORDER BY kind,id LIMIT $6`, scope.Tenant, scope.Subject, epoch, after.Kind, after.ID, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		result = storage.StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{}}
		for rows.Next() {
			var row storage.StreamSourceRow
			if err := rows.Scan(&row.Kind, &row.ID, &row.Size, &row.SHA256); err != nil {
				return err
			}
			if row.Validate() != nil || !sourceKind(row.Kind) {
				return ErrPrivateSource
			}
			if len(result.Rows) == limit {
				last := result.Rows[len(result.Rows)-1]
				result.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
				break
			}
			result.Rows = append(result.Rows, row)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return result.Validate(scope, epoch, after)
	})
	return result, err
}

// OpenPreflightJSON returns one exact retained legacy row. Its commitment is
// already immutable; a source change is rejected rather than presented as a
// different page of the original plan. Original bytes never use this port.
func (p *PrivateSource) OpenPreflightJSON(ctx context.Context, scope storage.Scope, epoch, ticket, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	if !model.SourceKind(kind) || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
		return storage.StreamSourceRow{}, nil, ErrPrivateSource
	}
	var descriptor storage.StreamSourceRow
	var raw []byte
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		if err := verifyPhotoPreflight(ctx, tx, scope, epoch, ticket); err != nil {
			return err
		}
		descriptor.Kind, descriptor.ID = kind, id
		if err := tx.QueryRow(ctx, `SELECT size_bytes,sha256,media_payload FROM photos_private_storage_preflight_inventory WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND kind=$4 AND id=$5`, scope.Tenant, scope.Subject, epoch, kind, id).Scan(&descriptor.Size, &descriptor.SHA256, &raw); err != nil {
			return err
		}
		if descriptor.Validate() != nil || descriptor.Size > model.MaxRowBytes {
			return ErrPrivateSource
		}
		if kind != "media" {
			query := `SELECT CASE WHEN octet_length(payload)<=$5 THEN payload END FROM (` + photosPrivateInventorySQL + `) retained WHERE kind=$3 AND id=$4`
			if err := tx.QueryRow(ctx, query, scope.Tenant, scope.Subject, kind, id, model.MaxRowBytes).Scan(&raw); err != nil {
				return err
			}
		}
		sum := sha256.Sum256(raw)
		if int64(len(raw)) != descriptor.Size || hex.EncodeToString(sum[:]) != descriptor.SHA256 || model.ValidateRow(scope, model.Row{Kind: kind, ID: id, JSON: raw}) != nil {
			return ErrPrivateSource
		}
		return ctx.Err()
	})
	if err != nil {
		clear(raw)
		return storage.StreamSourceRow{}, nil, err
	}
	return descriptor, &photoPreflightReader{Reader: bytes.NewReader(raw), raw: raw}, nil
}

type photoPreflightReader struct {
	Reader *bytes.Reader
	raw    []byte
	mu     sync.Mutex
	closed bool
}

func (r *photoPreflightReader) Read(out []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	return r.Reader.Read(out)
}

func (r *photoPreflightReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	clear(r.raw)
	r.raw = nil
	return nil
}
