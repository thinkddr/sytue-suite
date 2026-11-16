// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

var (
	ErrPrivateStorageFrozen = errors.New("erp: legacy storage is read-only after migration starts")
	ErrPrivateStorageEpoch  = errors.New("erp: retained source does not match the migration")
)

const privateERPStateTable = "erp_private_storage_states"

func PrivateStorageError(err error) error {
	var failure *pgconn.PgError
	if errors.As(err, &failure) && failure.Code == "PST01" {
		return ErrPrivateStorageFrozen
	}
	return err
}

func privateERPScope(ctx context.Context, owner string) (storage.Scope, error) {
	tenant, err := tenancy.From(ctx)
	scope := storage.Scope{Tenant: tenant.Slug, Subject: owner, Product: "erp"}
	if err != nil || !scope.Valid() {
		return scope, pg.ErrPrivateStorageScope
	}
	return scope, nil
}

func (s *Store) privateERPTx(ctx context.Context, owner string, fn func(storage.Scope, pgx.Tx) error) error {
	scope, err := privateERPScope(ctx, owner)
	if err != nil {
		return err
	}
	if s == nil || s.pool == nil {
		return pg.ErrPrivateStorageScope
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, scope.Subject); err != nil {
			return err
		}
		return fn(scope, tx)
	})
}

func privateERPMigrationState(scope storage.Scope, state pg.PrivateStorageState) (storage.AppMigrationState, error) {
	result := storage.AppMigrationState{
		Version: 1,
		LegacyMigrationState: storage.LegacyMigrationState{
			Scope:     scope,
			Mode:      state.Mode,
			Epoch:     state.Epoch,
			StartedAt: state.StartedAt,
			Rows:      state.Rows,
			Blobs:     state.Blobs,
		},
	}
	return result, result.Validate(scope)
}

func privateERPKind(kind string) bool {
	switch kind {
	case "record", "activity":
		return true
	default:
		return false
	}
}

func privateERPSourceError(err error) error {
	if errors.Is(err, pg.ErrPrivateSource) || errors.Is(err, pg.ErrPrivateSnapshotEpoch) {
		return ErrPrivateStorageEpoch
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoEncontrado
	}
	return err
}

func (s *Store) PrivateStorageState(ctx context.Context, owner string) (storage.AppMigrationState, error) {
	var result storage.AppMigrationState
	err := s.privateERPTx(ctx, owner, func(scope storage.Scope, tx pgx.Tx) error {
		state, err := pg.ReadPrivateStorageState(ctx, tx, privateERPStateTable, scope.Tenant, scope.Subject)
		if err != nil {
			return err
		}
		result, err = privateERPMigrationState(scope, state)
		return err
	})
	return result, privateERPSourceError(err)
}

func (s *Store) BeginPrivateStorageMigration(ctx context.Context, owner string, epoch uuid.UUID) (storage.AppMigrationState, error) {
	if epoch == uuid.Nil {
		return storage.AppMigrationState{}, ErrPrivateStorageEpoch
	}
	var result storage.AppMigrationState
	err := s.privateERPTx(ctx, owner, func(scope storage.Scope, tx pgx.Tx) error {
		state, err := pg.BeginPrivateStorageSnapshot(ctx, tx, pg.PrivateSnapshotConfig{
			Scope: scope,
			Epoch: epoch,
			Capture: func(ctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
				var rows int64
				if err := tx.QueryRow(ctx, `SELECT public.erp_freeze_private_storage($1,$2,$3)`, scope.Tenant, scope.Subject, epoch).Scan(&rows); err != nil {
					return pg.PrivateSnapshotCounts{}, err
				}
				return pg.PrivateSnapshotCounts{Rows: rows}, nil
			},
		})
		if err != nil {
			return err
		}
		result, err = privateERPMigrationState(scope, state)
		return err
	})
	return result, privateERPSourceError(err)
}

func (s *Store) PrivateStorageSourcePage(ctx context.Context, owner string, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	var result storage.StreamSourcePage
	err := s.privateERPTx(ctx, owner, func(scope storage.Scope, tx pgx.Tx) error {
		var err error
		result, err = pg.ReadPrivateStorageSourcePage(ctx, tx, pg.PrivateSourceConfig{
			Scope: scope,
			Epoch: epoch,
			Kind:  privateERPKind,
		}, after, limit)
		return err
	})
	return result, privateERPSourceError(err)
}

func (s *Store) OpenPrivateStorageSource(ctx context.Context, owner string, epoch uuid.UUID, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	var body pg.PrivateSourceBody
	err := s.privateERPTx(ctx, owner, func(scope storage.Scope, tx pgx.Tx) error {
		var err error
		body, err = pg.ReadPrivateStorageSourceBody(ctx, tx, pg.PrivateSourceConfig{
			Scope: scope,
			Epoch: epoch,
			Kind:  privateERPKind,
		}, kind, id)
		return err
	})
	if err != nil {
		return storage.StreamSourceRow{}, nil, privateERPSourceError(err)
	}
	reader, err := body.OpenJSON(ctx)
	return body.Descriptor, reader, err
}
