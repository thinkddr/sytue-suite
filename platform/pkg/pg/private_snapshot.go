// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var (
	ErrPrivateSnapshotEpoch     = errors.New("pg: private snapshot epoch does not match")
	ErrPrivateSnapshotIsolation = errors.New("pg: private snapshot capture requires read committed")
	ErrPrivateSnapshotCapture   = errors.New("pg: private snapshot capture is incomplete")
)

type PrivateSnapshotCounts struct{ Rows, Blobs int64 }

// PrivateSnapshotConfig supplies domain work inside the existing writer fence.
// Capture must run collaboration/ingress checks and verify complete retained
// data before returning counts. An error requires rollback of the caller's tx.
// Product registry determines the state table; no caller-supplied SQL identifier
// or new fence protocol is accepted.
type PrivateSnapshotConfig struct {
	Scope   storage.Scope
	Epoch   uuid.UUID
	Account *uuid.UUID
	Capture func(context.Context, pgx.Tx) (PrivateSnapshotCounts, error)
}

// BeginPrivateStorageSnapshot serializes capture with the existing FOR SHARE
// writer guard and publishes frozen only after successful domain validation.
// The caller owns the tenant transaction. A repeatable snapshot could predate a
// writer that only locked (without updating) the state row; it is rejected,
// never silently downgraded to another isolation level.
func BeginPrivateStorageSnapshot(ctx context.Context, tx pgx.Tx, cfg PrivateSnapshotConfig) (PrivateStorageState, error) {
	if tx == nil || !cfg.Scope.Valid() || cfg.Epoch == uuid.Nil || cfg.Capture == nil || (cfg.Account != nil && *cfg.Account == uuid.Nil) {
		return PrivateStorageState{}, ErrPrivateStorageScope
	}
	table := cfg.Scope.Product + "_private_storage_states"
	name, err := privateStorageTable(ctx, table, cfg.Scope.Tenant, cfg.Scope.Subject)
	if err != nil {
		return PrivateStorageState{}, err
	}
	var isolation string
	if err := tx.QueryRow(ctx, "SHOW transaction_isolation").Scan(&isolation); err != nil {
		return PrivateStorageState{}, err
	}
	if isolation != "read committed" {
		return PrivateStorageState{}, ErrPrivateSnapshotIsolation
	}
	if err := EnsurePrivateStorageState(ctx, tx, table, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Account); err != nil {
		return PrivateStorageState{}, err
	}
	state, err := LockPrivateStorageState(ctx, tx, table, cfg.Scope.Tenant, cfg.Scope.Subject)
	if err != nil {
		return state, err
	}
	if err := checkPrivateSnapshotAccount(ctx, tx, cfg, name); err != nil {
		return state, err
	}
	if err := validatePrivateSnapshotState(cfg.Scope, state); err != nil {
		return state, err
	}
	if state.Mode != "legacy" {
		if (state.Mode != "frozen" && state.Mode != "private") || state.Epoch != cfg.Epoch.String() {
			return state, ErrPrivateSnapshotEpoch
		}
		return state, nil
	}
	return capturePrivateSnapshot(ctx, tx, cfg, name, state)
}

func checkPrivateSnapshotAccount(ctx context.Context, tx pgx.Tx, cfg PrivateSnapshotConfig, name string) error {
	if cfg.Account == nil {
		return nil
	}
	var account uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT account_id FROM `+name+` WHERE tenant=$1 AND owner_sub=$2`, cfg.Scope.Tenant, cfg.Scope.Subject).Scan(&account); err != nil {
		return err
	}
	if account != *cfg.Account {
		return ErrPrivateStorageScope
	}
	return nil
}

func capturePrivateSnapshot(ctx context.Context, tx pgx.Tx, cfg PrivateSnapshotConfig, name string, state PrivateStorageState) (PrivateStorageState, error) {
	var started time.Time
	err := tx.QueryRow(ctx, `UPDATE `+name+` SET mode='building',epoch=$3,started_at=clock_timestamp() WHERE tenant=$1 AND owner_sub=$2 AND mode='legacy' RETURNING started_at`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Epoch).Scan(&started)
	if err != nil {
		return state, err
	}
	counts, err := cfg.Capture(ctx, tx)
	if err != nil {
		return state, err
	}
	candidate := PrivateStorageState{Mode: "frozen", Epoch: cfg.Epoch.String(), StartedAt: &started, Rows: counts.Rows, Blobs: counts.Blobs}
	if err := validatePrivateSnapshotState(cfg.Scope, candidate); err != nil {
		return state, err
	}
	updated, err := tx.Exec(ctx, `UPDATE `+name+` SET mode='frozen',source_rows=$4,source_blobs=$5 WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND mode='building' AND started_at=$6`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Epoch, counts.Rows, counts.Blobs, started)
	if err != nil {
		return state, err
	}
	if updated.RowsAffected() != 1 {
		return state, ErrPrivateSnapshotCapture
	}
	return ReadPrivateStorageState(ctx, tx, cfg.Scope.Product+"_private_storage_states", cfg.Scope.Tenant, cfg.Scope.Subject)
}

func validatePrivateSnapshotState(scope storage.Scope, state PrivateStorageState) error {
	value := storage.LegacyMigrationState{Scope: scope, Mode: state.Mode, Epoch: state.Epoch, StartedAt: state.StartedAt, Rows: state.Rows, Blobs: state.Blobs}
	if value.Validate(scope) != nil {
		return ErrPrivateSnapshotCapture
	}
	return nil
}
