// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// EnsureMatrixDraftRooms serializes with identity lifecycle changes. The server
// stores only references: draft content and keys stay in Matrix clients.
func (s *Store) EnsureMatrixDraftRooms(ctx context.Context, subject string, provision func(context.Context, string) (string, error)) ([]string, error) {
	return s.ensureMatrixPrivateRooms(ctx, subject, "", provision)
}

// EnsureMatrixStorageRooms shares the identity gate and lifecycle of drafts,
// with a separate namespace so old clients cannot mistake object data for drafts.
func (s *Store) EnsureMatrixStorageRooms(ctx context.Context, subject, product string, provision func(context.Context, string) (string, error)) ([]string, error) {
	if !domain.StorageProduct(product) {
		return nil, domain.ErrInvalidInput
	}
	return s.ensureMatrixPrivateRooms(ctx, subject, product, provision)
}

//nolint:gocognit // serialized private-room provisioning retains the identity gate and RLS transaction.
func (s *Store) ensureMatrixPrivateRooms(ctx context.Context, subject, product string, provision func(context.Context, string) (string, error)) ([]string, error) {
	tenant := tenancy.MustFrom(ctx)
	selectRooms := `SELECT room_id FROM matrix_draft_rooms WHERE tenant=$1 AND subject=$2 ORDER BY created_at DESC,room_id`
	insertRoom := `INSERT INTO matrix_draft_rooms(tenant,subject,room_id) VALUES($1,$2,$3)`
	if product != "" {
		selectRooms = `SELECT room_id FROM matrix_storage_rooms WHERE tenant=$1 AND subject=$2 AND product=$3 ORDER BY created_at DESC,room_id`
		insertRoom = `INSERT INTO matrix_storage_rooms(tenant,subject,room_id,product) VALUES($1,$2,$3,$4)`
	}
	rooms := []string{}
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		scoped := matrixContext(ctx, tx)
		return s.WithMatrixIdentity(scoped, subject, func(ctx context.Context) error {
			var denied bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_tenant_access WHERE tenant=$1 AND subject=$2 AND status<>'active')`, tenant.Slug, subject).Scan(&denied); err != nil {
				return err
			}
			if denied {
				return domain.ErrForbidden
			}
			args := []any{tenant.Slug, subject}
			if product != "" {
				args = append(args, product)
			}
			rows, err := tx.Query(ctx, selectRooms, args...)
			if err != nil {
				return err
			}
			for rows.Next() {
				var room string
				if err := rows.Scan(&room); err != nil {
					rows.Close()
					return err
				}
				rooms = append(rooms, room)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			if len(rooms) == 0 {
				room, err := provision(ctx, "")
				if err != nil {
					return err
				}
				args = []any{tenant.Slug, subject, room}
				if product != "" {
					args = append(args, product)
				}
				if _, err := tx.Exec(ctx, insertRoom, args...); err != nil {
					return err
				}
				rooms = append(rooms, room)
				return nil
			}
			for _, room := range rooms {
				if _, err := provision(ctx, room); err != nil {
					return err
				}
			}
			return nil
		})
	})
	return rooms, err
}

type matrixPrivateBinding struct{ tenant, room, table string }

// Revocation removes an actor from private draft and application rooms. Other
// tenants remain accessible after organization-only removal.
func (s *Store) revokeMatrixPrivateRooms(ctx context.Context, subject, tenant string) error {
	var bindings []matrixPrivateBinding
	if err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT tenant,room_id,'matrix_draft_rooms' FROM matrix_draft_rooms WHERE subject=$1 AND ($2='' OR tenant=$2)
 UNION ALL SELECT tenant,room_id,'matrix_storage_rooms' FROM matrix_storage_rooms WHERE subject=$1 AND ($2='' OR tenant=$2) ORDER BY room_id`, subject, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b matrixPrivateBinding
			if err := rows.Scan(&b.tenant, &b.room, &b.table); err != nil {
				return err
			}
			bindings = append(bindings, b)
		}
		return rows.Err()
	}); err != nil {
		return err
	}
	for _, b := range bindings {
		if err := s.revokeMatrixPrivateRoom(ctx, subject, b); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) revokeMatrixPrivateRoom(ctx context.Context, subject string, b matrixPrivateBinding) error {
	if b.table != "matrix_draft_rooms" && b.table != "matrix_storage_rooms" {
		return domain.ErrInvalidInput
	}
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, b.tenant); err != nil {
			return err
		}
		query := `SELECT room_id FROM matrix_draft_rooms WHERE room_id=$1 AND tenant=$2 AND subject=$3 FOR UPDATE`
		if b.table == "matrix_storage_rooms" {
			query = `SELECT room_id FROM matrix_storage_rooms WHERE room_id=$1 AND tenant=$2 AND subject=$3 FOR UPDATE`
		}
		var room string
		if err := tx.QueryRow(ctx, query, b.room, b.tenant, subject).Scan(&room); err != nil {
			return err
		}
		var denied bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_identity_access WHERE subject=$1 AND status<>'active') OR EXISTS(SELECT 1 FROM matrix_tenant_access WHERE tenant=$2 AND subject=$1 AND status<>'active')`, subject, b.tenant).Scan(&denied); err != nil {
			return err
		}
		if !denied {
			return nil
		}
		if s.matrixGate == nil {
			return domain.ErrMatrixUnavailable
		}
		return s.matrixGate.Reconcile(ctx, room, nil, false, false)
	})
}
