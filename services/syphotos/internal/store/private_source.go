// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// PrivateSource is a retained legacy source, not a key owner or media worker.
// The caller mounts it only with the complete SQL/S3 fence and the authenticated
// encrypted-plan control plane. Private originals never enter this server port.
type PrivateSource struct {
	store   *Store
	objects retainedVariantStore
}

func NewPrivateSource(st *Store, objects *objectstore.Client) (*PrivateSource, error) {
	if st == nil || st.pool == nil || objects == nil {
		return nil, ErrPrivateSource
	}
	return &PrivateSource{store: st, objects: objects}, nil
}

func (p *PrivateSource) ownerTx(ctx context.Context, scope storage.Scope, run func(pgx.Tx) error) error {
	tenant, err := tenancy.From(ctx)
	if err != nil || !scope.Valid() || scope.Product != "photos" || tenant.Slug != scope.Tenant || p == nil || p.store == nil || p.objects == nil {
		return pg.ErrPrivateStorageScope
	}
	return p.store.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, scope.Subject); err != nil {
			return err
		}
		return run(tx)
	})
}

func rawPhotoState(scope storage.Scope, state pg.PrivateStorageState) storage.AppMigrationState {
	return storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: state.Mode, Epoch: state.Epoch, StartedAt: state.StartedAt, Rows: state.Rows, Blobs: state.Blobs}}
}

func (p *PrivateSource) State(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	var result storage.AppMigrationState
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		state, err := pg.ReadPrivateStorageState(ctx, tx, "photos_private_storage_states", scope.Tenant, scope.Subject)
		if err != nil {
			return err
		}
		result = rawPhotoState(scope, state)
		if state.Mode == "legacy" {
			if err := tx.QueryRow(ctx, photoSourceSharedSQL, scope.Tenant, scope.Subject).Scan(&result.Shared); err != nil {
				return err
			}
		}
		return result.Validate(scope)
	})
	return result, err
}

// These indexed existence checks mirror PersonalCompatible without reading
// EXIF/vectors or opening S3 while the caller only asks for routing metadata.
const photoSourceSharedSQL = `SELECT
 EXISTS(SELECT 1 FROM albums a WHERE a.tenant=$1
  AND(a.owner_sub=$2 OR EXISTS(SELECT 1 FROM album_personas p WHERE p.tenant=a.tenant AND p.album_id=a.id AND p.sujeto=$2))
  AND(a.owner_sub<>$2 OR a.public_token<>'' OR a.public_token_hash<>''
   OR EXISTS(SELECT 1 FROM album_personas p WHERE p.tenant=a.tenant AND p.album_id=a.id)))
 OR EXISTS(SELECT 1 FROM assets a WHERE a.tenant=$1 AND a.owner_sub<>$2 AND(
  EXISTS(SELECT 1 FROM album_assets m JOIN albums al ON al.tenant=m.tenant AND al.id=m.album_id WHERE m.tenant=a.tenant AND m.asset_id=a.id AND al.owner_sub=$2)
  OR EXISTS(SELECT 1 FROM albums al WHERE al.tenant=a.tenant AND al.cover_asset_id=a.id AND al.owner_sub=$2)
  OR EXISTS(SELECT 1 FROM edits e WHERE e.tenant=a.tenant AND e.asset_id=a.id AND e.owner_sub=$2)))
 OR EXISTS(SELECT 1 FROM edits e JOIN assets a ON a.tenant=e.tenant AND a.id=e.asset_id
  WHERE e.tenant=$1 AND a.owner_sub=$2 AND e.owner_sub<>$2)`

func lockPhotoLegacySource(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
	var isolation string
	if err := tx.QueryRow(ctx, "SHOW transaction_isolation").Scan(&isolation); err != nil {
		return err
	}
	if isolation != "read committed" {
		return pg.ErrPrivateSnapshotIsolation
	}
	if err := pg.EnsurePrivateStorageState(ctx, tx, "photos_private_storage_states", scope.Tenant, scope.Subject, nil); err != nil {
		return err
	}
	state, err := pg.LockPrivateStorageState(ctx, tx, "photos_private_storage_states", scope.Tenant, scope.Subject)
	if err != nil {
		return err
	}
	if state.Mode != "legacy" {
		return ErrPrivateSource
	}
	return nil
}

type photoPreflight struct {
	ticket, inventory      string
	rows, blobs, originals int64
	room, event            *string
}

func readPhotoPreflight(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch string) (photoPreflight, error) {
	var row photoPreflight
	err := tx.QueryRow(ctx, `SELECT ticket::text,inventory_sha256,backend_rows,backend_blobs,original_blobs,plan_room_id,plan_event_id FROM photos_private_storage_preflights WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3`, scope.Tenant, scope.Subject, epoch).Scan(&row.ticket, &row.inventory, &row.rows, &row.blobs, &row.originals, &row.room, &row.event)
	return row, err
}

func (c capturedPhotoSource) matches(row photoPreflight) bool {
	return c.SHA256 == row.inventory && int64(len(c.Descriptors)) == row.rows && int64(len(c.Variants)) == row.blobs && c.Originals == row.originals
}

func (p *PrivateSource) Preflight(ctx context.Context, scope storage.Scope, epoch string) (model.SourcePreflight, error) {
	if !storage.MigrationEpoch(epoch) {
		return model.SourcePreflight{}, ErrPrivateSource
	}
	var result model.SourcePreflight
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		if err := lockPhotoLegacySource(ctx, tx, scope); err != nil {
			return err
		}
		capture, err := capturePhotoSource(ctx, tx, p.objects, scope, epoch)
		if err != nil {
			return err
		}
		defer capture.clear()
		row, err := readPhotoPreflight(ctx, tx, scope, epoch)
		if errors.Is(err, pgx.ErrNoRows) {
			row = photoPreflight{ticket: uuid.NewString(), inventory: capture.SHA256, rows: int64(len(capture.Descriptors)), blobs: int64(len(capture.Variants)), originals: capture.Originals}
			_, err = tx.Exec(ctx, `INSERT INTO photos_private_storage_preflights(tenant,owner_sub,epoch,ticket,inventory_sha256,backend_rows,backend_blobs,original_blobs)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, scope.Tenant, scope.Subject, epoch, row.ticket, row.inventory, row.rows, row.blobs, row.originals)
		}
		if err != nil {
			return err
		}
		if !capture.matches(row) {
			return ErrPrivateSource
		}
		result = model.SourcePreflight{Scope: scope, Epoch: epoch, CAS: row.ticket, Rows: capture.Rows, Backend: capture.Descriptors, BackendBlobs: row.blobs}
		capture.Rows = nil
		return result.Validate(scope, epoch)
	})
	return result, err
}

func photoPlanRouting(ctx context.Context, tx pgx.Tx, scope storage.Scope) (model.SourcePlanRouting, error) {
	result := model.SourcePlanRouting{Scope: scope}
	rows, err := tx.Query(ctx, `SELECT epoch::text,plan_room_id,plan_event_id FROM photos_private_storage_preflights WHERE tenant=$1 AND owner_sub=$2 AND plan_event_id IS NOT NULL LIMIT 2`, scope.Tenant, scope.Subject)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		if result.Plan != nil {
			return model.SourcePlanRouting{}, ErrPrivateSource
		}
		var plan model.SourcePlanReference
		if err := rows.Scan(&plan.Epoch, &plan.Link.RoomID, &plan.Link.EventID); err != nil {
			return result, err
		}
		result.Plan = &plan
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	return result, result.Validate(scope)
}

func (p *PrivateSource) PlanRouting(ctx context.Context, scope storage.Scope) (model.SourcePlanRouting, error) {
	var result model.SourcePlanRouting
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error { var err error; result, err = photoPlanRouting(ctx, tx, scope); return err })
	return result, err
}

func (p *PrivateSource) AcceptPlan(ctx context.Context, scope storage.Scope, epoch, ticket string, link storage.Reference) (model.SourcePlanReference, error) {
	if !storage.MigrationEpoch(epoch) || !storage.MigrationEpoch(ticket) || !link.Valid() {
		return model.SourcePlanReference{}, ErrPrivateSource
	}
	result := model.SourcePlanReference{Epoch: epoch, Link: link}
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		if err := lockPhotoLegacySource(ctx, tx, scope); err != nil {
			return err
		}
		// A rollback locks the parent first. Take the eventual write lock now
		// (before any AccessShare upgrade), then protect inventory through the
		// complete capture and commit. If rollback already removed it, fail.
		if _, err := tx.Exec(ctx, `LOCK TABLE photos_private_storage_preflights IN ROW EXCLUSIVE MODE;
 LOCK TABLE photos_private_storage_preflight_inventory IN ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		routing, err := photoPlanRouting(ctx, tx, scope)
		if err != nil {
			return err
		}
		if routing.Plan != nil && *routing.Plan != result {
			return ErrPrivateSource
		}
		row, err := readPhotoPreflight(ctx, tx, scope, epoch)
		if err != nil {
			return err
		}
		if row.ticket != ticket {
			return ErrPrivateSource
		}
		capture, err := capturePhotoSourceBounded(ctx, tx, p.objects, scope, epoch)
		if err != nil {
			return err
		}
		if !capture.matches(row) {
			return ErrPrivateSource
		}
		if _, err := retainPhotoPreflight(ctx, tx, scope, epoch, capture); err != nil {
			return err
		}
		updated, err := tx.Exec(ctx, `UPDATE photos_private_storage_preflights SET plan_room_id=$4,plan_event_id=$5 WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND ticket=$6 AND (plan_room_id IS NULL OR (plan_room_id=$4 AND plan_event_id=$5))`, scope.Tenant, scope.Subject, epoch, link.RoomID, link.EventID, ticket)
		if err != nil {
			return err
		}
		if updated.RowsAffected() != 1 {
			return ErrPrivateSource
		}
		return nil
	})
	return result, err
}

func (p *PrivateSource) Begin(ctx context.Context, scope storage.Scope, epoch uuid.UUID) (storage.AppMigrationState, error) {
	if epoch == uuid.Nil {
		return storage.AppMigrationState{}, ErrPrivateSource
	}
	var result storage.AppMigrationState
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		routing, err := photoPlanRouting(ctx, tx, scope)
		if err != nil {
			return err
		}
		if routing.Plan == nil || routing.Plan.Epoch != epoch.String() {
			return ErrPrivateSource
		}
		state, err := pg.BeginPrivateStorageSnapshot(ctx, tx, pg.PrivateSnapshotConfig{Scope: scope, Epoch: epoch, Capture: func(ctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
			row, err := readPhotoPreflight(ctx, tx, scope, epoch.String())
			if err != nil {
				return pg.PrivateSnapshotCounts{}, err
			}
			if row.room == nil || row.event == nil || *row.room != routing.Plan.Link.RoomID || *row.event != routing.Plan.Link.EventID {
				return pg.PrivateSnapshotCounts{}, ErrPrivateSource
			}
			capture, err := capturePhotoSourceBounded(ctx, tx, p.objects, scope, epoch.String())
			if err != nil {
				return pg.PrivateSnapshotCounts{}, err
			}
			if !capture.matches(row) {
				return pg.PrivateSnapshotCounts{}, ErrPrivateSource
			}
			if err := insertPhotoSource(ctx, tx, scope, epoch); err != nil {
				return pg.PrivateSnapshotCounts{}, err
			}
			return pg.PrivateSnapshotCounts{Rows: row.rows, Blobs: row.blobs}, nil
		}})
		if err != nil {
			return err
		}
		result = rawPhotoState(scope, state)
		return result.Validate(scope)
	})
	return result, err
}

func insertPhotoSource(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO photos_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload,size_bytes,sha256,storage_key)
 SELECT $1,$2,$3,kind,id,payload,size_bytes,sha256,storage_key FROM pg_temp.sytue_photos_capture`, scope.Tenant, scope.Subject, epoch)
	return err
}

func (p *PrivateSource) Page(ctx context.Context, scope storage.Scope, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	var result storage.StreamSourcePage
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		result, err = pg.ReadPrivateStorageSourcePage(ctx, tx, pg.PrivateSourceConfig{Scope: scope, Epoch: epoch, Kind: sourceKind}, after, limit)
		return err
	})
	return result, err
}

func validateVariantBody(ctx context.Context, tx pgx.Tx, cfg pg.PrivateSourceConfig, body pg.PrivateSourceBody) error {
	parts := strings.Split(body.StorageKey, "/")
	if len(parts) != 4 {
		return ErrPrivateSource
	}
	key, err := retainedVariantKey(parts[1], body.StorageKey)
	if err != nil {
		return err
	}
	id := sha256.Sum256([]byte(body.StorageKey))
	if body.Descriptor.Kind != "variant" || body.Descriptor.ID != hex.EncodeToString(id[:]) {
		return ErrPrivateSource
	}
	metadata, err := pg.ReadPrivateStorageSourceBody(ctx, tx, cfg, "media", body.Descriptor.ID)
	if err != nil {
		return err
	}
	reader, err := metadata.OpenJSON(ctx)
	if err != nil {
		return err
	}
	defer reader.Close()
	raw, err := io.ReadAll(io.LimitReader(reader, 8<<10+1))
	if err != nil {
		return err
	}
	defer clear(raw)
	var media model.MediaRow
	if len(raw) > 8<<10 || json.Unmarshal(raw, &media) != nil || media.Validate() != nil || media.Tenant != cfg.Scope.Tenant || media.Owner != cfg.Scope.Subject || media.Descriptor() != body.Descriptor || media.AssetID != parts[1] || media.Fingerprint != key.Huella || media.Variant != key.Variante || media.Format != key.Formato {
		return ErrPrivateSource
	}
	var owned bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM photos_private_storage_snapshot WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND kind='asset' AND id=$4 AND convert_from(payload,'UTF8')::jsonb->>'id'=$4 AND convert_from(payload,'UTF8')::jsonb->>'tenant'=$1 AND convert_from(payload,'UTF8')::jsonb->>'owner_sub'=$2)`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Epoch, media.AssetID).Scan(&owned)
	if err != nil {
		return err
	}
	if !owned {
		return ErrPrivateSource
	}
	return nil
}

func (p *PrivateSource) Open(ctx context.Context, scope storage.Scope, epoch uuid.UUID, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	var body pg.PrivateSourceBody
	var output io.ReadCloser
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		cfg := pg.PrivateSourceConfig{Scope: scope, Epoch: epoch, Kind: sourceKind}
		var err error
		body, err = pg.ReadPrivateStorageSourceBody(ctx, tx, cfg, kind, id)
		if err != nil {
			return err
		}
		if body.StorageKey != "" {
			return validateVariantBody(ctx, tx, cfg, body)
		}
		output, err = body.OpenJSON(ctx)
		return err
	})
	if err != nil {
		if output != nil {
			_ = output.Close()
		}
		return storage.StreamSourceRow{}, nil, err
	}
	if output == nil {
		var err error
		output, err = p.objects.Get(ctx, tenancy.Tenant{Slug: scope.Tenant}, body.StorageKey)
		if err != nil {
			return storage.StreamSourceRow{}, nil, err
		}
	}
	if output == nil {
		return storage.StreamSourceRow{}, nil, ErrPrivateSource
	}
	return body.Descriptor, output, nil
}
