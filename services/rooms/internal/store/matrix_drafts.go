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
//
//nolint:gocognit // serialized draft provisioning with identity gates and RLS checks requires nested callbacks
func (s *Store) EnsureMatrixDraftRooms(ctx context.Context, subject string, provision func(context.Context, string) (string, error)) ([]string, error) {
	tenant := tenancy.MustFrom(ctx)
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
			rows, err := tx.Query(ctx, `SELECT room_id FROM matrix_draft_rooms WHERE tenant=$1 AND subject=$2 ORDER BY created_at DESC,room_id`, tenant.Slug, subject)
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
				if _, err := tx.Exec(ctx, `INSERT INTO matrix_draft_rooms(tenant,subject,room_id) VALUES($1,$2,$3)`, tenant.Slug, subject, room); err != nil {
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

// Revocation also removes an actor's devices from private draft rooms. Other
// tenants remain accessible after organization-only removal.
func (s *Store) revokeMatrixDraftRooms(ctx context.Context, subject, tenant string) error {
	type binding struct{ tenant, room string }
	var bindings []binding
	if err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT tenant,room_id FROM matrix_draft_rooms WHERE subject=$1 AND ($2='' OR tenant=$2) ORDER BY room_id`, subject, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b binding
			if err := rows.Scan(&b.tenant, &b.room); err != nil {
				return err
			}
			bindings = append(bindings, b)
		}
		return rows.Err()
	}); err != nil {
		return err
	}
	for _, b := range bindings {
		if err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(ctx, tx, b.tenant); err != nil {
				return err
			}
			var room string
			if err := tx.QueryRow(ctx, `SELECT room_id FROM matrix_draft_rooms WHERE room_id=$1 AND tenant=$2 AND subject=$3 FOR UPDATE`, b.room, b.tenant, subject).Scan(&room); err != nil {
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
		}); err != nil {
			return err
		}
	}
	return nil
}
