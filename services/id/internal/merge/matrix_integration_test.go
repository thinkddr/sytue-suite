// SPDX-License-Identifier: Apache-2.0
//go:build integration

package merge

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	roomsdb "git.sytue.eu/sytue/one/services/rooms/db"
	"github.com/jackc/pgx/v5"
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
