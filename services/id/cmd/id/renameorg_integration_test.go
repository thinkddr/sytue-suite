// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	iddb "git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/merge"
	"git.sytue.eu/sytue/one/services/id/internal/store"
	roomsdb "git.sytue.eu/sytue/one/services/rooms/db"
)

func renameSuiteFixture(t *testing.T) (*pgtest.Harness, map[string]*pgtest.Harness, string) {
	t.Helper()
	t.Setenv("SYTUE_ID_ROOMS_CALL_ACCESS_SECRET", "")
	minimal := fstest.MapFS{"migrations/0001_empty.up.sql": &fstest.MapFile{Data: []byte(`SELECT 1;`)}}
	parent := pgtest.New(t, minimal, "migrations")
	identity := parent.Sibling(t, "id", iddb.Migrations, iddb.MigrationsDir)
	org := uuid.NewString()
	if _, err := identity.Pool.Exec(t.Context(), `INSERT INTO organizations(id,slug,name) VALUES($1,'source','Retained identity')`, org); err != nil {
		t.Fatal(err)
	}
	services := make(map[string]*pgtest.Harness, len(merge.Services))
	for _, service := range merge.Services {
		var h *pgtest.Harness
		if service == "rooms" {
			h = parent.Sibling(t, service, roomsdb.Migrations, roomsdb.MigrationsDir)
		} else {
			h = parent.Sibling(t, service, minimal, "migrations")
		}
		services[service] = h
		if _, err := h.PoolSinRLS(t).Exec(t.Context(), `CREATE TABLE rename_domain_rows(tenant text NOT NULL,payload text NOT NULL);
ALTER TABLE rename_domain_rows ENABLE ROW LEVEL SECURITY;
ALTER TABLE rename_domain_rows FORCE ROW LEVEL SECURITY;
CREATE POLICY rename_domain_merge ON rename_domain_rows USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
INSERT INTO rename_domain_rows VALUES('source','retained source');`); err != nil {
			t.Fatal(err)
		}
	}
	return identity, services, org
}

func TestRenameOrgScopePreflightPrecedesEveryServiceMove(t *testing.T) {
	identity, services, org := renameSuiteFixture(t)
	ctx := t.Context()
	st := store.New(identity.Pool)
	if _, err := services["rooms"].PoolSinRLS(t).Exec(ctx, `INSERT INTO matrix_storage_rooms(tenant,subject,product,room_id) VALUES('source','alice','mail','!original:matrix.test')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := renameOrg(ctx, st, identity.AppDSN, "source", "target"); err == nil {
			t.Fatal("renamed a retained encrypted Scope")
		}
	}
	for service, h := range services {
		var n int
		if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM rename_domain_rows WHERE tenant='source' AND payload='retained source'`).Scan(&n); err != nil || n != 1 {
			t.Fatal("moved domain before scope preflight", service, n, err)
		}
	}
	var actual string
	if err := identity.Pool.QueryRow(ctx, `SELECT slug FROM organizations WHERE id=$1`, org).Scan(&actual); err != nil || actual != "source" {
		t.Fatal("preflight changed identity", actual, err)
	}
	intent, err := st.FindOrgRename(ctx, "source", "target")
	if err != nil || intent.OrgID != org || intent.Status != "pending" {
		t.Fatal("retry lost retained intent", intent, err)
	}
	if _, err := identity.Pool.Exec(ctx, `INSERT INTO organizations(id,slug,name) VALUES($1,'target','Other organization')`, uuid.NewString()); err == nil {
		t.Fatal("pending intent lost its target reservation")
	}
}

func TestRenameOrgCompletesOwnedAdmissionAndPreservesNewTargetCiphertextOnRetry(t *testing.T) {
	identity, services, org := renameSuiteFixture(t)
	ctx := t.Context()
	st := store.New(identity.Pool)
	moved, err := renameOrg(ctx, st, identity.AppDSN, "source", "target")
	if err != nil || moved != len(merge.Services) {
		t.Fatal("owned rename", moved, err)
	}
	var actual string
	if err := identity.Pool.QueryRow(ctx, `SELECT slug FROM organizations WHERE id=$1`, org).Scan(&actual); err != nil || actual != "target" {
		t.Fatal("identity not committed", actual, err)
	}
	for service, h := range services {
		var n int
		if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM rename_domain_rows WHERE tenant='target' AND payload='retained source'`).Scan(&n); err != nil || n != 1 {
			t.Fatal("domain not retained", service, n, err)
		}
	}
	if err := services["rooms"].Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "target"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO matrix_storage_rooms(tenant,subject,product,room_id) VALUES('target','alice','mail','!new-target:matrix.test')`)
		return err
	}); err != nil {
		t.Fatal("completed target remained closed", err)
	}
	if _, err := renameOrg(ctx, st, identity.AppDSN, "source", "target"); !errors.Is(err, errRenameYaHecho) {
		t.Fatal("retry failed to reconcile committed identity", err)
	}
	var room string
	if err := services["rooms"].PoolSinRLS(t).QueryRow(ctx, `SELECT room_id FROM matrix_storage_rooms WHERE tenant='target'`).Scan(&room); err != nil || room != "!new-target:matrix.test" {
		t.Fatal("retry touched new target ciphertext", room, err)
	}
	if _, err := identity.Pool.Exec(ctx, `INSERT INTO organizations(id,slug,name) VALUES($1,'source','Other organization')`, uuid.NewString()); err == nil {
		t.Fatal("retired source slug was reassigned")
	}
	intent, err := st.FindOrgRename(ctx, "source", "target")
	if err != nil || intent.Status != "completed" {
		t.Fatal("completion not retained", intent, err)
	}
}
