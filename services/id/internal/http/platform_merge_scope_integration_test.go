// SPDX-License-Identifier: Apache-2.0
//go:build integration

package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/internal/merge"
	"git.sytue.eu/sytue/one/services/id/internal/store"
	roomsdb "git.sytue.eu/sytue/one/services/rooms/db"
)

func TestExecuteMergeScopePreflightLeavesEveryServiceAndIdentityUnchanged(t *testing.T) {
	for _, boundTenant := range []string{"source", "target"} {
		t.Run(boundTenant, func(t *testing.T) {
			identity, services := mergeScopeSuiteFixture(t)
			roomsAdmin := services["rooms"].PoolSinRLS(t)
			if _, err := roomsAdmin.Exec(t.Context(), `INSERT INTO matrix_storage_rooms(tenant,subject,product,room_id) VALUES($1,'alice','photos','!retained:fixture.invalid')`, boundTenant); err != nil {
				t.Fatal(err)
			}
			sourceID, targetID := uuid.NewString(), uuid.NewString()
			if _, err := identity.Pool.Exec(t.Context(), `INSERT INTO organizations(id) VALUES($1),($2)`, sourceID, targetID); err != nil {
				t.Fatal(err)
			}
			id := uuid.NewString()
			if _, err := identity.Pool.Exec(t.Context(), `INSERT INTO org_merges(id) VALUES($1)`, id); err != nil {
				t.Fatal(err)
			}
			server := &Server{cfg: Config{DatabaseDSN: identity.AppDSN}, store: store.New(identity.Pool), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			err := server.executeMerge(t.Context(), id, "source", "target", sourceID, targetID)
			if err == nil {
				t.Fatal("scope-bound Matrix mapping did not block the suite")
			}
			if !strings.Contains(err.Error(), "rooms") || !strings.Contains(err.Error(), "matrix_storage_rooms") {
				t.Fatal("preflight lost the responsible scope", err)
			}
			for service, harness := range services {
				var n int
				if err := harness.PoolSinRLS(t).QueryRow(t.Context(), `SELECT count(*) FROM untouched_domain_rows WHERE (tenant='source' AND payload='exact source') OR (tenant='target' AND payload='exact target')`).Scan(&n); err != nil || n != 2 {
					t.Fatal("suite moved a domain before detecting scope-bound data", service, n, err)
				}
			}
			var progress string
			if err := identity.Pool.QueryRow(t.Context(), `SELECT services::text FROM org_merges WHERE id=$1`, id).Scan(&progress); err != nil || progress != "{}" {
				t.Fatal("preflight touched service progress before rejecting", progress, err)
			}
			var active int
			if err := identity.Pool.QueryRow(t.Context(), `SELECT count(*) FROM organizations WHERE status='active' AND merged_into_org_id IS NULL`).Scan(&active); err != nil || active != 2 {
				t.Fatal("preflight changed identity", active, err)
			}
			var tenant, room string
			if err := roomsAdmin.QueryRow(t.Context(), `SELECT tenant,room_id FROM matrix_storage_rooms`).Scan(&tenant, &room); err != nil || tenant != boundTenant || room != "!retained:fixture.invalid" {
				t.Fatal("scope-bound reference was retagged", tenant, room, err)
			}
		})
	}
}

func mergeScopeSuiteFixture(t *testing.T) (*pgtest.Harness, map[string]*pgtest.Harness) {
	t.Helper()
	minimal := fstest.MapFS{"migrations/0001_init.up.sql": &fstest.MapFile{Data: []byte(`
CREATE TABLE org_merges(id uuid PRIMARY KEY,source_org_id uuid,target_org_id uuid,status text NOT NULL DEFAULT 'pending',error text NOT NULL DEFAULT '',services jsonb NOT NULL DEFAULT '{}',started_at timestamptz,finished_at timestamptz,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE organizations(id uuid PRIMARY KEY,slug text NOT NULL DEFAULT '',status text NOT NULL DEFAULT 'active',merged_into_org_id uuid);
`)}}
	parent := pgtest.New(t, minimal, "migrations")
	identity := parent.Sibling(t, "id", minimal, "migrations")
	services := make(map[string]*pgtest.Harness, len(merge.Services))
	for _, service := range merge.Services {
		var harness *pgtest.Harness
		if service == "rooms" {
			harness = parent.Sibling(t, service, roomsdb.Migrations, roomsdb.MigrationsDir)
		} else {
			harness = parent.Sibling(t, service, minimal, "migrations")
		}
		services[service] = harness
		admin := harness.PoolSinRLS(t)
		if _, err := admin.Exec(t.Context(), `CREATE TABLE untouched_domain_rows(tenant text NOT NULL,payload text NOT NULL);
ALTER TABLE untouched_domain_rows ENABLE ROW LEVEL SECURITY;
ALTER TABLE untouched_domain_rows FORCE ROW LEVEL SECURITY;
CREATE POLICY untouched_domain_merge ON untouched_domain_rows USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
INSERT INTO untouched_domain_rows VALUES('source','exact source'),('target','exact target')`); err != nil {
			t.Fatal(err)
		}
	}
	return identity, services
}

func TestExecuteMergeReconcilesCommittedIdentityWithoutMovingNewTargetCiphertext(t *testing.T) {
	identity, services := mergeScopeSuiteFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	sourceID, targetID, job := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := identity.Pool.Exec(ctx, `INSERT INTO organizations(id) VALUES($1),($2);`, sourceID, targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Pool.Exec(ctx, `INSERT INTO org_merges(id) VALUES($1)`, job); err != nil {
		t.Fatal(err)
	}
	admission, err := merge.PrepareAdmission(ctx, identity.AppDSN, job, "source", "target")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: Config{DatabaseDSN: identity.AppDSN}, store: store.New(identity.Pool), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	uncertain := server.settleFailedMergeAdmission(ctx, admission, uuid.NewString(), targetID, errors.New("identity outcome unavailable"))
	if uncertain == nil || !strings.Contains(uncertain.Error(), "admission remains closed") {
		t.Fatal("unknown identity outcome reopened admission", uncertain)
	}
	blocked := services["rooms"].Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "source"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO matrix_storage_rooms(tenant,subject,product,room_id) VALUES('source','unknown-outcome','photos','!blocked:fixture.invalid')`)
		return err
	})
	var closed *pgconn.PgError
	if !errors.As(blocked, &closed) || closed.Code != "PST03" {
		t.Fatal("unknown identity decision admitted source ciphertext", blocked)
	}
	// Model a completed domain phase and an ID COMMIT whose ACK was lost.
	for _, h := range services {
		if _, err := h.PoolSinRLS(t).Exec(ctx, `UPDATE untouched_domain_rows SET tenant='target' WHERE tenant='source'`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := identity.Pool.Exec(ctx, `UPDATE organizations SET status='merged',merged_into_org_id=$2 WHERE id=$1`, sourceID, targetID); err != nil {
		t.Fatal(err)
	}
	// Failure settlement must finish, rather than abort/reopen, the durable pair.
	original := errors.New("identity commit acknowledgement lost")
	if err := server.settleFailedMergeAdmission(ctx, admission, sourceID, targetID, original); !errors.Is(err, original) {
		t.Fatal("lost original failure", err)
	}
	rooms := services["rooms"]
	if err := rooms.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "target"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO matrix_storage_rooms(tenant,subject,product,room_id) VALUES('target','new-target-device','photos','!new-ciphertext:fixture.invalid')`)
		return err
	}); err != nil {
		t.Fatal("target generation did not reopen", err)
	}
	if err := server.executeMerge(ctx, job, "source", "target", sourceID, targetID); err != nil {
		t.Fatal("committed merge retry rescanned or moved target ciphertext", err)
	}
	var count int
	if err := rooms.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM matrix_storage_rooms WHERE tenant='target' AND room_id='!new-ciphertext:fixture.invalid'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("retry touched new ciphertext mapping", count, err)
	}
	err = rooms.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "source"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO matrix_storage_rooms(tenant,subject,product,room_id) VALUES('source','retired-device','photos','!must-not-resurrect:fixture.invalid')`)
		return err
	})
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "PST03" {
		t.Fatal("lost ACK recovery reopened retired source", err)
	}
	for service, h := range services {
		if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM untouched_domain_rows WHERE tenant='target'`).Scan(&count); err != nil || count != 2 {
			t.Fatal("recovery moved an already completed domain", service, count, err)
		}
	}
	var progress string
	if err := identity.Pool.QueryRow(ctx, `SELECT services->>'identity' FROM org_merges WHERE id=$1`, job).Scan(&progress); err != nil || progress != "completed" {
		t.Fatal("recovery did not reconcile identity progress", progress, err)
	}
}
