// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

var ErrPrivateSource = errors.New("cast: retained source changed or is incomplete")

type RetainedObjects interface {
	List(context.Context, tenancy.Tenant, string, string, int) (objectstore.ObjectPage, error)
	Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error)
	GetRange(context.Context, tenancy.Tenant, string, string) (*objectstore.Objeto, error)
}

// PrivateSource reads retained historical rows and exact media. It never owns
// encryption keys, transcodes, deletes sources or asserts native UI readiness.
type PrivateSource struct {
	store   *Store
	objects RetainedObjects
}

func NewPrivateSource(st *Store, objects RetainedObjects) (*PrivateSource, error) {
	if st == nil || st.pool == nil || objects == nil {
		return nil, ErrPrivateSource
	}
	return &PrivateSource{store: st, objects: objects}, nil
}

func (p *PrivateSource) ownerTx(ctx context.Context, scope storage.Scope, run func(pgx.Tx) error) error {
	tenant, err := tenancy.From(ctx)
	if err != nil || !scope.Valid() || scope.Product != "cast" || tenant.Slug != scope.Tenant || p == nil || p.store == nil || p.objects == nil {
		return pg.ErrPrivateStorageScope
	}
	return p.store.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true),set_config('app.cast_persona',$1,true)`, scope.Subject); err != nil {
			return err
		}
		return run(tx)
	})
}

func castMigrationState(scope storage.Scope, state pg.PrivateStorageState) storage.AppMigrationState {
	return storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: state.Mode, Epoch: state.Epoch, StartedAt: state.StartedAt, Rows: state.Rows, Blobs: state.Blobs}}
}

func (p *PrivateSource) State(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	var result storage.AppMigrationState
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		state, err := pg.ReadPrivateStorageState(ctx, tx, "cast_private_storage_states", scope.Tenant, scope.Subject)
		if err != nil {
			return err
		}
		result = castMigrationState(scope, state)
		if state.Mode == "legacy" {
			snapshot, err := ReadPrivateStorageInventory(ctx, tx, scope)
			if err != nil {
				return err
			}
			result.Shared = errors.Is(snapshot.PersonalCompatible(), model.ErrShared)
			var unsupported bool
			if err := tx.QueryRow(ctx, `SELECT public.cast_private_unmodeled($1,$2)`, scope.Tenant, scope.Subject).Scan(&unsupported); err != nil {
				return err
			}
			result.Shared = result.Shared || unsupported
		}
		return result.Validate(scope)
	})
	return result, err
}

func (p *PrivateSource) Begin(ctx context.Context, scope storage.Scope, epoch uuid.UUID) (storage.AppMigrationState, error) {
	if epoch == uuid.Nil {
		return storage.AppMigrationState{}, ErrPrivateSource
	}
	var result storage.AppMigrationState
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		// Drain global blocks/contributions as well as this tenant's owner writers.
		if _, err := tx.Exec(ctx, `SELECT public.cast_private_subject_freeze($1)`, scope.Subject); err != nil {
			return err
		}
		state, err := pg.BeginPrivateStorageSnapshot(ctx, tx, pg.PrivateSnapshotConfig{Scope: scope, Epoch: epoch, Capture: func(ctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
			var unsupported bool
			if err := tx.QueryRow(ctx, `SELECT public.cast_private_unmodeled($1,$2)`, scope.Tenant, scope.Subject).Scan(&unsupported); err != nil {
				return pg.PrivateSnapshotCounts{}, err
			}
			if unsupported {
				return pg.PrivateSnapshotCounts{}, model.ErrShared
			}
			return p.capture(ctx, tx, scope, epoch)
		}})
		if err != nil {
			return err
		}
		result = castMigrationState(scope, state)
		return result.Validate(scope)
	})
	return result, err
}
func sourceKind(kind string) bool { return kind == model.BinaryKind || model.SourceKind(kind) }
func (p *PrivateSource) Page(ctx context.Context, scope storage.Scope, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	var result storage.StreamSourcePage
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		result, err = pg.ReadPrivateStorageSourcePage(ctx, tx, pg.PrivateSourceConfig{Scope: scope, Epoch: epoch, Kind: sourceKind}, after, limit)
		return err
	})
	return result, err
}

func validateCastMedia(ctx context.Context, tx pgx.Tx, cfg pg.PrivateSourceConfig, body pg.PrivateSourceBody) error {
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
	defer clear(raw)
	if err != nil || len(raw) > 8<<10 {
		return ErrPrivateSource
	}
	var media model.MediaRow
	if json.Unmarshal(raw, &media) != nil || media.Owner != cfg.Scope.Subject || media.Tenant != cfg.Scope.Tenant || media.Key != body.StorageKey || media.Descriptor() != body.Descriptor || !strings.HasPrefix(media.Key, media.VideoID+"/") {
		return ErrPrivateSource
	}
	_, err = model.RetainedRow("media", raw)
	return err
}

func (p *PrivateSource) Open(ctx context.Context, scope storage.Scope, epoch uuid.UUID, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	var body pg.PrivateSourceBody
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		cfg := pg.PrivateSourceConfig{Scope: scope, Epoch: epoch, Kind: sourceKind}
		var err error
		body, err = pg.ReadPrivateStorageSourceBody(ctx, tx, cfg, kind, id)
		if err != nil {
			return err
		}
		if body.StorageKey != "" {
			return validateCastMedia(ctx, tx, cfg, body)
		}
		return nil
	})
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	if body.StorageKey == "" {
		reader, err := body.OpenJSON(ctx)
		return body.Descriptor, reader, err
	}
	reader, err := p.objects.Get(ctx, tenancy.Tenant{Slug: scope.Tenant}, body.StorageKey)
	if err != nil {
		if reader != nil {
			_ = reader.Close()
		}
		return storage.StreamSourceRow{}, nil, err
	}
	if reader == nil {
		return storage.StreamSourceRow{}, nil, ErrPrivateSource
	}
	return body.Descriptor, reader, nil
}
