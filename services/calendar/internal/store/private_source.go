// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

var ErrPrivateStorageEpoch = errors.New("calendar: retained source does not match the migration")

func (s *Store) privateCalendarTx(ctx context.Context, owner string, operation func(privateCalendarIdentity, pgx.Tx) error) error {
	identity, err := verifiedCalendarIdentity(ctx, owner)
	if err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, identity.scope.Subject); err != nil {
			return err
		}
		return operation(identity, tx)
	})
}

func (s *Store) PrivateStorageState(ctx context.Context, owner string) (storage.AppMigrationState, error) {
	var result storage.AppMigrationState
	err := s.privateCalendarTx(ctx, owner, func(identity privateCalendarIdentity, tx pgx.Tx) error {
		state, err := pg.ReadPrivateStorageState(ctx, tx, "calendar_private_storage_states", identity.scope.Tenant, identity.scope.Subject)
		if err != nil {
			return err
		}
		compat, err := privateCalendarPreflight(ctx, tx, identity)
		if err != nil {
			return err
		}
		result = storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: identity.scope, Mode: state.Mode, Epoch: state.Epoch, StartedAt: state.StartedAt, Rows: state.Rows, Blobs: state.Blobs}, Shared: compat.Collaboration || compat.Resources, MaxBodyBytes: model.MaxRowBytes}
		return result.Validate(identity.scope)
	})
	return result, err
}

// Begin remains closed until the common inbox/executor supplies a real verified
// device ingress bridge. A frozen historical source or an environment boolean
// cannot grant that capability. Compatibility is read-only and never provisions
// a substitute empty calendar or closes part of the legacy account.
func (s *Store) BeginPrivateStorageMigration(ctx context.Context, owner string, epoch uuid.UUID) (storage.AppMigrationState, error) {
	if _, err := verifiedCalendarIdentity(ctx, owner); err != nil {
		return storage.AppMigrationState{}, err
	}
	if epoch == uuid.Nil {
		return storage.AppMigrationState{}, ErrPrivateStorageEpoch
	}
	compat, err := s.PrivateStorageCompatibility(ctx, owner)
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	if err := compat.Err(); err != nil {
		return storage.AppMigrationState{}, err
	}
	return storage.AppMigrationState{}, ErrPrivateStorageIngress
}

// stagePrivateStorageSnapshot prepares the complete retained source under the
// production fence, but has no public route or caller until continuing ingress
// is implemented. Its caller owns this transaction and must roll back on error.
func stagePrivateStorageSnapshot(ctx context.Context, tx pgx.Tx, identity privateCalendarIdentity, epoch uuid.UUID) (pg.PrivateStorageState, error) {
	verified, err := verifiedCalendarIdentity(ctx, identity.scope.Subject)
	if err != nil || verified.scope != identity.scope || !slices.Equal(verified.addresses, identity.addresses) || tx == nil {
		return pg.PrivateStorageState{}, pg.ErrPrivateStorageScope
	}
	if epoch == uuid.Nil {
		return pg.PrivateStorageState{}, ErrPrivateStorageEpoch
	}
	if _, err := tx.Exec(ctx, `SELECT calendar_private_lock_identities($1,$2),set_config('app.owner_sub',$1,true)`, identity.scope.Subject, identity.addresses); err != nil {
		return pg.PrivateStorageState{}, err
	}
	state, err := pg.BeginPrivateStorageSnapshot(ctx, tx, pg.PrivateSnapshotConfig{Scope: identity.scope, Epoch: epoch, Capture: func(ctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
		return capturePrivateCalendarRows(ctx, tx, identity, epoch)
	}})
	if err != nil {
		return state, err
	}
	var addresses []string
	if err := tx.QueryRow(ctx, `SELECT coalesce(array_agg(address ORDER BY address COLLATE "C"),ARRAY[]::text[]) FROM calendar_private_storage_aliases WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3`, identity.scope.Tenant, identity.scope.Subject, epoch).Scan(&addresses); err != nil {
		return state, err
	}
	if !slices.Equal(addresses, identity.addresses) {
		return state, pg.ErrPrivateStorageScope
	}
	return state, nil
}

func capturePrivateCalendarRows(ctx context.Context, tx pgx.Tx, identity privateCalendarIdentity, epoch uuid.UUID) (pg.PrivateSnapshotCounts, error) {
	compat, err := privateCalendarPreflight(ctx, tx, identity)
	if err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	if err := compat.Err(); err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	snapshot, err := ReadPrivateStorageInventory(ctx, tx, identity.scope)
	if err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	rows := snapshot.Rows()
	if len(rows) == 0 {
		return pg.PrivateSnapshotCounts{}, domain.ErrNotFound
	}
	captured, err := tx.Exec(ctx, `WITH retained_inventory AS (`+calendarPrivateInventorySQL+`)
INSERT INTO calendar_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload)
SELECT $1,$2,$3,kind,id,payload FROM retained_inventory`, identity.scope.Tenant, identity.scope.Subject, epoch)
	if err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	if captured.RowsAffected() != int64(len(rows)) {
		return pg.PrivateSnapshotCounts{}, pg.ErrPrivateSnapshotCapture
	}
	kinds, ids, hashes := make([]string, len(rows)), make([]string, len(rows)), make([]string, len(rows))
	for i, row := range rows {
		sum := sha256.Sum256(row.JSON)
		kinds[i], ids[i], hashes[i] = row.Kind, row.ID, hex.EncodeToString(sum[:])
		clear(row.JSON)
	}
	var verifiedRows int64
	err = tx.QueryRow(ctx, `SELECT count(*) FROM calendar_private_storage_snapshot s JOIN unnest($4::text[],$5::text[],$6::text[]) h(kind,id,sha256)ON s.kind=h.kind AND s.id=h.id AND s.sha256=h.sha256 WHERE s.tenant=$1 AND s.owner_sub=$2 AND s.epoch=$3`, identity.scope.Tenant, identity.scope.Subject, epoch, kinds, ids, hashes).Scan(&verifiedRows)
	if err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	if verifiedRows != captured.RowsAffected() {
		return pg.PrivateSnapshotCounts{}, pg.ErrPrivateSnapshotCapture
	}
	if _, err := tx.Exec(ctx, `INSERT INTO calendar_private_storage_aliases(tenant,owner_sub,epoch,address)SELECT $1,$2,$3,address FROM unnest($4::text[])a(address)`, identity.scope.Tenant, identity.scope.Subject, epoch, identity.addresses); err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	return pg.PrivateSnapshotCounts{Rows: verifiedRows}, nil
}

func calendarPrivateSourceError(err error) error {
	if errors.Is(err, pg.ErrPrivateSource) || errors.Is(err, pg.ErrPrivateSnapshotEpoch) {
		return ErrPrivateStorageEpoch
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func (s *Store) PrivateStorageSourcePage(ctx context.Context, owner string, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	var result storage.StreamSourcePage
	err := s.privateCalendarTx(ctx, owner, func(identity privateCalendarIdentity, tx pgx.Tx) error {
		var err error
		result, err = pg.ReadPrivateStorageSourcePage(ctx, tx, pg.PrivateSourceConfig{Scope: identity.scope, Epoch: epoch, Kind: model.SourceKind}, after, limit)
		return err
	})
	return result, calendarPrivateSourceError(err)
}

func (s *Store) OpenPrivateStorageSource(ctx context.Context, owner string, epoch uuid.UUID, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	var body pg.PrivateSourceBody
	err := s.privateCalendarTx(ctx, owner, func(identity privateCalendarIdentity, tx pgx.Tx) error {
		var err error
		body, err = pg.ReadPrivateStorageSourceBody(ctx, tx, pg.PrivateSourceConfig{Scope: identity.scope, Epoch: epoch, Kind: model.SourceKind}, kind, id)
		return err
	})
	if err != nil {
		return storage.StreamSourceRow{}, nil, calendarPrivateSourceError(err)
	}
	reader, err := body.OpenJSON(ctx)
	return body.Descriptor, reader, err
}
