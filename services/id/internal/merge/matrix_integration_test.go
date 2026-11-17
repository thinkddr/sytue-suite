// SPDX-License-Identifier: Apache-2.0
//go:build integration

package merge

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	roomsdb "git.sytue.eu/sytue/one/services/rooms/db"
)

func TestRoomsMergeKeepsNewestMatrixLifecycleTombstone(t *testing.T) {
	h := pgtest.New(t, roomsdb.Migrations, roomsdb.MigrationsDir)
	ctx := context.Background()
	if err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO matrix_tenant_access(tenant,subject,status,version,applied) VALUES
   ('org-absorbida','same','removed',40,true),('org-receptora','same','active',39,true),
   ('org-absorbida','other','active',42,true),('org-receptora','other','removed',43,true)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	result, err := MoveService(ctx, h.Pool, origen, destino)
	if err != nil {
		t.Fatal(err)
	}
	if result.Prepared == "" {
		t.Fatal("deduplication did not report retained lifecycle versions")
	}
	if err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT subject,status,version FROM matrix_tenant_access WHERE tenant=$1 ORDER BY subject`, destino)
		if err != nil {
			return err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var subject, status string
			var version int64
			if err := rows.Scan(&subject, &status, &version); err != nil {
				return err
			}
			if status != "removed" || (subject == "same" && version != 40) || (subject == "other" && version != 43) {
				t.Fatalf("lost newest denial %s/%s/%d", subject, status, version)
			}
			n++
		}
		if n != 2 {
			t.Fatalf("rows=%d", n)
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	leftovers, err := LeftoversIn(ctx, h.Pool, "rooms", origen)
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("leftovers=%v err=%v", leftovers, err)
	}
}

func TestRoomsStorageMappingsRemainImmutableUnderMaintenance(t *testing.T) {
	h := pgtest.New(t, roomsdb.Migrations, roomsdb.MigrationsDir)
	ctx := t.Context()
	admin := h.PoolSinRLS(t)
	for _, table := range []string{"matrix_storage_rooms", "matrix_storage_isolated_rooms"} {
		if _, err := admin.Exec(ctx, `INSERT INTO `+pgx.Identifier{table}.Sanitize()+`(tenant,subject,product,room_id) VALUES('org-absorbida','alice','photos',$1)`, "!"+table+":fixture.invalid"); err != nil {
			t.Fatal(err)
		}
		err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE `+pgx.Identifier{table}.Sanitize()+` SET tenant='org-receptora' WHERE tenant='org-absorbida'`)
			return err
		})
		var blocked *pgconn.PgError
		if !errors.As(err, &blocked) || blocked.Code != "PST01" {
			t.Fatal("maintenance retagged an encrypted record scope", table, err)
		}
	}
}

func TestRoomsEncryptedDraftScopeBlocksBeforeDiscardingLoginGrants(t *testing.T) {
	for _, tenant := range []string{origen, destino} {
		t.Run(tenant, func(t *testing.T) {
			h := pgtest.New(t, roomsdb.Migrations, roomsdb.MigrationsDir)
			admin := h.PoolSinRLS(t)
			if _, err := admin.Exec(t.Context(), `INSERT INTO matrix_draft_rooms(tenant,subject,room_id) VALUES($1,'alice','!retained-draft:fixture.invalid')`, tenant); err != nil {
				t.Fatal(err)
			}
			if _, err := admin.Exec(t.Context(), `INSERT INTO matrix_native_consents(tenant,state_hash,subject,challenge,expires_at) VALUES($1,'synthetic-state','alice',repeat('A',43),now()+interval '2 minutes')`, origen); err != nil {
				t.Fatal(err)
			}
			_, err := MoveService(t.Context(), h.Pool, origen, destino)
			if !errors.Is(err, pg.ErrScopeBoundMerge) {
				t.Fatal("tenant-bound encrypted draft did not block merge", err)
			}
			var n int
			if err := admin.QueryRow(t.Context(), `SELECT count(*) FROM matrix_native_consents WHERE tenant=$1 AND state_hash='synthetic-state'`, origen).Scan(&n); err != nil || n != 1 {
				t.Fatal("prepare hook discarded grants before scope preflight", n, err)
			}
			if err := admin.QueryRow(t.Context(), `SELECT count(*) FROM matrix_draft_rooms WHERE tenant=$1 AND room_id='!retained-draft:fixture.invalid'`, tenant).Scan(&n); err != nil || n != 1 {
				t.Fatal("encrypted draft scope was retagged", n, err)
			}
		})
	}
}
