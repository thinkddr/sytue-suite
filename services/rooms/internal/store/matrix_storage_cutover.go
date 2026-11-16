// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func cutoverScope(ctx context.Context, subject, product string) (storage.Scope, error) {
	tenant, err := tenancy.From(ctx)
	scope := storage.Scope{Tenant: tenant.Slug, Subject: subject, Product: product}
	if err != nil || !scope.Valid() {
		return storage.Scope{}, domain.ErrInvalidInput
	}
	return scope, nil
}

func legacyStorageRoomsTx(ctx context.Context, tx pgx.Tx, scope storage.Scope) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT room_id FROM matrix_storage_rooms WHERE tenant=$1 AND subject=$2 AND product=$3 ORDER BY created_at DESC,room_id`, scope.Tenant, scope.Subject, scope.Product)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rooms := []string{}
	for rows.Next() {
		var room string
		if err := rows.Scan(&room); err != nil {
			return nil, err
		}
		rooms = append(rooms, room)
	}
	return rooms, rows.Err()
}

func storageCutoverTx(ctx context.Context, tx pgx.Tx, scope storage.Scope) (storage.IdentityCutoverState, error) {
	result := storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: "legacy", SourceRooms: []string{}}
	var room, event *string
	err := tx.QueryRow(ctx, `SELECT epoch::text,phase,source_rooms,receipt_room,receipt_event FROM matrix_storage_cutovers WHERE tenant=$1 AND subject=$2 AND product=$3 FOR UPDATE`, scope.Tenant, scope.Subject, scope.Product).Scan(&result.Epoch, &result.Phase, &result.SourceRooms, &room, &event)
	if errors.Is(err, pgx.ErrNoRows) {
		result.SourceRooms, err = legacyStorageRoomsTx(ctx, tx, scope)
	}
	if err != nil {
		return result, err
	}
	if room != nil && event != nil {
		result.Receipt = &storage.Reference{RoomID: *room, EventID: *event}
	}
	if result.Validate(scope) != nil {
		return result, domain.ErrConflict
	}
	return result, nil
}

// withStorageCutover serializes with the SAME identity/tenant gate as room
// provisioning and revocation; callbacks reuse its existing transaction.
func (s *Store) withStorageCutover(ctx context.Context, subject, product string, fn func(context.Context, pgx.Tx, storage.Scope) error) error {
	scope, err := cutoverScope(ctx, subject, product)
	if err != nil {
		return err
	}
	return s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		return s.WithMatrixIdentity(matrixContext(ctx, tx), subject, func(scoped context.Context) error { return fn(scoped, tx, scope) })
	})
}

func (s *Store) MatrixStorageCutover(ctx context.Context, subject, product string) (storage.IdentityCutoverState, error) {
	var state storage.IdentityCutoverState
	err := s.withStorageCutover(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		var err error
		state, err = storageCutoverTx(ctx, tx, scope)
		return err
	})
	return state, err
}

// Begin persists freezing BEFORE any remote Matrix update. A failure or process
// restart cannot restore a writable legacy bootstrap; retry uses the same epoch.
func (s *Store) BeginMatrixStorageCutover(ctx context.Context, subject, product, epoch string) (storage.IdentityCutoverState, error) {
	if !storage.MigrationEpoch(epoch) {
		return storage.IdentityCutoverState{}, domain.ErrInvalidInput
	}
	var state storage.IdentityCutoverState
	err := s.withStorageCutover(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		var err error
		state, err = storageCutoverTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		if state.Phase != "legacy" {
			if state.Epoch != epoch {
				return domain.ErrConflict
			}
			return nil
		}
		state.Phase, state.Epoch = "freezing", epoch
		_, err = tx.Exec(ctx, `INSERT INTO matrix_storage_cutovers(tenant,subject,product,epoch,phase,source_rooms) VALUES($1,$2,$3,$4,'freezing',$5)`, scope.Tenant, scope.Subject, scope.Product, epoch, state.SourceRooms)
		return err
	})
	return state, err
}

func checkCutoverSourceTx(ctx context.Context, tx pgx.Tx, state storage.IdentityCutoverState) error {
	rooms, err := legacyStorageRoomsTx(ctx, tx, state.Scope)
	if err != nil {
		return err
	}
	if !slices.Equal(rooms, state.SourceRooms) {
		return domain.ErrConflict
	}
	return nil
}

type FreezeStorageRoom func(context.Context, string) error

// Freeze runs after Begin committed. Its durable freezing row also survives a
// partial remote failure; no source contents or encryption material are read.
func (s *Store) FreezeMatrixStorageCutover(ctx context.Context, subject, product, epoch string, freeze FreezeStorageRoom) (storage.IdentityCutoverState, error) {
	if !storage.MigrationEpoch(epoch) || freeze == nil {
		return storage.IdentityCutoverState{}, domain.ErrInvalidInput
	}
	var state storage.IdentityCutoverState
	err := s.withStorageCutover(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		var err error
		state, err = storageCutoverTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		if state.Phase == "legacy" || state.Epoch != epoch {
			return domain.ErrConflict
		}
		if err := freezeCutoverSource(ctx, tx, state, freeze); err != nil {
			return err
		}
		if state.Phase != "freezing" {
			return nil
		}
		_, err = tx.Exec(ctx, `UPDATE matrix_storage_cutovers SET phase='frozen' WHERE tenant=$1 AND subject=$2 AND product=$3`, scope.Tenant, scope.Subject, scope.Product)
		if err == nil {
			state.Phase = "frozen"
		}
		return err
	})
	return state, err
}

// Complete accepts only an encrypted event in this actor/product's isolated
// room. The server validates public sender/routing, never decrypts the receipt.
// Original rooms remain retained and read-only; this operation deletes nothing.
func (s *Store) CompleteMatrixStorageCutover(ctx context.Context, subject, product, epoch string, receipt storage.Reference, freeze FreezeStorageRoom, validate func(context.Context) error) (storage.IdentityCutoverState, error) {
	if !storage.MigrationEpoch(epoch) || !receipt.Valid() || freeze == nil || validate == nil {
		return storage.IdentityCutoverState{}, domain.ErrInvalidInput
	}
	var state storage.IdentityCutoverState
	err := s.withStorageCutover(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		var err error
		state, err = storageCutoverTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		state, err = completeCutoverTx(ctx, tx, state, epoch, receipt, freeze, validate)
		return err
	})
	return state, err
}

func freezeCutoverSource(ctx context.Context, tx pgx.Tx, state storage.IdentityCutoverState, freeze FreezeStorageRoom) error {
	if err := checkCutoverSourceTx(ctx, tx, state); err != nil {
		return err
	}
	pending, err := matrixLegacyPublicationPendingTx(ctx, tx, state.Scope, state.SourceRooms)
	if err != nil {
		return err
	}
	if pending {
		return domain.ErrConflict
	}
	for _, room := range state.SourceRooms {
		if err := freeze(ctx, room); err != nil {
			return err
		}
	}
	return nil
}

func completeCutoverTx(ctx context.Context, tx pgx.Tx, state storage.IdentityCutoverState, epoch string, receipt storage.Reference, freeze FreezeStorageRoom, validate func(context.Context) error) (storage.IdentityCutoverState, error) {
	if state.Epoch != epoch {
		return state, domain.ErrConflict
	}
	if state.Phase == "isolated" {
		if state.Receipt == nil || *state.Receipt != receipt {
			return state, domain.ErrConflict
		}
		return state, nil
	}
	if state.Phase != "frozen" {
		return state, domain.ErrConflict
	}
	scope := state.Scope
	var owns bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_storage_isolated_rooms WHERE tenant=$1 AND subject=$2 AND product=$3 AND room_id=$4)`, scope.Tenant, scope.Subject, scope.Product, receipt.RoomID).Scan(&owns); err != nil {
		return state, err
	}
	if !owns {
		return state, domain.ErrForbidden
	}
	if err := freezeCutoverSource(ctx, tx, state, freeze); err != nil {
		return state, err
	}
	if err := validate(ctx); err != nil {
		return state, err
	}
	_, err := tx.Exec(ctx, `UPDATE matrix_storage_cutovers SET phase='isolated',receipt_room=$4,receipt_event=$5 WHERE tenant=$1 AND subject=$2 AND product=$3`, scope.Tenant, scope.Subject, scope.Product, receipt.RoomID, receipt.EventID)
	if err == nil {
		state.Phase = "isolated"
		state.Receipt = &receipt
	}
	return state, err
}

type privateRoomPolicy struct {
	frozen bool
	rooms  []string
}

func privateRoomSource(ctx context.Context, tx pgx.Tx, scope storage.Scope, version storage.IdentityVersion) (privateRoomPolicy, error) {
	if scope.Product == "" || version != storage.LegacyIdentity {
		return privateRoomPolicy{}, nil
	}
	state, err := storageCutoverTx(ctx, tx, scope)
	return privateRoomPolicy{frozen: state.Phase != "legacy", rooms: state.SourceRooms}, err
}

func privateRoomSourceMatches(source privateRoomPolicy, rooms []string) bool {
	return !source.frozen || len(rooms) > 0 && slices.Equal(rooms, source.rooms)
}
