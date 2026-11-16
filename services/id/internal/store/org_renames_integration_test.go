// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func renameIntentFixture(t *testing.T) (*pgtest.Harness, *store.Store, string) {
	t.Helper()
	migration, err := fs.ReadFile(db.Migrations, "migrations/0121_org_rename_intents.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrations := fstest.MapFS{
		"migrations/0001_initial.up.sql": &fstest.MapFile{Data: []byte(`CREATE TABLE organizations(id uuid PRIMARY KEY,slug text NOT NULL UNIQUE,status text NOT NULL DEFAULT 'active',updated_at timestamptz NOT NULL DEFAULT now());`)},
		"migrations/0002_rename.up.sql":  &fstest.MapFile{Data: migration},
	}
	h := pgtest.New(t, migrations, "migrations")
	org := uuid.NewString()
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO organizations(id,slug) VALUES($1,'source')`, org); err != nil {
		t.Fatal(err)
	}
	return h, store.New(h.Pool), org
}

func ownedRenameStore(t *testing.T, h *pgtest.Harness, id string) *store.Store {
	t.Helper()
	session, err := pg.TrySessionLock(t.Context(), h.Pool, "organization-operation:"+id, pg.OrganizationOperationLockGroup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := session.Close(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	return store.New(h.Pool).WithDatabase(session)
}

func TestOrgRenameRetainsOriginalIdentityAndReservesBothSlugs(t *testing.T) {
	h, st, org := renameIntentFixture(t)
	ctx := t.Context()
	id := uuid.NewString()
	if _, err := st.PrepareOrgRename(ctx, id, "source", "target"); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("pool without owned session prepared a rename", err)
	}
	st = ownedRenameStore(t, h, id)
	r, err := st.PrepareOrgRename(ctx, id, "source", "target")
	if err != nil || r.OrgID != org || r.Status != "pending" {
		t.Fatal("prepare", r, err)
	}
	if _, err := st.PrepareOrgRename(ctx, id, "source", "target"); err != nil {
		t.Fatal("idempotent prepare", err)
	}
	if _, err := st.PrepareOrgRename(ctx, uuid.NewString(), "source", "target"); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("another job replaced retained intent", err)
	}
	if committed, err := st.OrgRenameIdentityOutcome(ctx, id); err != nil || committed {
		t.Fatal("pending identity reported committed", committed, err)
	}
	if _, err := h.Pool.Exec(ctx, `INSERT INTO organizations(id,slug) VALUES($1,'target')`, uuid.NewString()); err == nil {
		t.Fatal("pending target was assigned to another organization")
	}
	if err := st.CommitOrgRename(ctx, r); err != nil {
		t.Fatal(err)
	}
	if committed, err := st.OrgRenameIdentityOutcome(ctx, id); err != nil || !committed {
		t.Fatal("committed identity lost", committed, err)
	}
	for range 2 {
		if err := st.CompleteOrgRename(ctx, id); err != nil {
			t.Fatal("completion retry", err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO organizations(id,slug) VALUES(gen_random_uuid(),'source')`,
		`UPDATE organizations SET slug='source' WHERE slug='target'`,
		`DELETE FROM org_renames`,
		`UPDATE org_renames SET target_slug='other'`,
		`UPDATE org_slug_claims SET retired=false WHERE slug='source'`,
		`DELETE FROM org_slug_claims WHERE slug='source'`,
	} {
		if _, err := h.Pool.Exec(ctx, statement); err == nil {
			t.Fatal("retained identity/reservation changed", statement)
		}
	}
	rollback, err := fs.ReadFile(db.Migrations, "migrations/0121_org_rename_intents.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	if _, err := conn.Exec(ctx, string(rollback)); err == nil {
		t.Fatal("rollback discarded retained identity")
	}
	if _, err := conn.Exec(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	found, err := st.FindOrgRename(ctx, "source", "target")
	if err != nil || found.ID != id || found.Status != "completed" {
		t.Fatal("failed rollback changed intent", found, err)
	}
}

func TestOrgRenameTargetReservationRejectsOlderSnapshot(t *testing.T) {
	h, _, _ := renameIntentFixture(t)
	ctx := t.Context()
	id := uuid.NewString()
	st := ownedRenameStore(t, h, id)
	conn, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_slug_claims`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PrepareOrgRename(ctx, id, "source", "target"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO organizations(id,slug) VALUES($1,'target')`, uuid.NewString()); err == nil {
		t.Fatal("stale snapshot stole a reserved target")
	}
}

func TestOrgRenameIdentityOutcomeWaitsForCommitOrRollback(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commit], func(t *testing.T) {
			h, _, _ := renameIntentFixture(t)
			ctx := t.Context()
			id := uuid.NewString()
			st := ownedRenameStore(t, h, id)
			r, err := st.PrepareOrgRename(ctx, id, "source", "target")
			if err != nil {
				t.Fatal(err)
			}
			conn, err := pgx.Connect(ctx, h.DSN)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
			if _, err := tx.Exec(ctx, `UPDATE organizations SET slug='target' WHERE id=$1;`, r.OrgID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `UPDATE org_slug_claims SET retired=true WHERE slug='source'; UPDATE org_renames SET status='identity_committed'`); err != nil {
				t.Fatal(err)
			}
			type outcome struct {
				committed bool
				err       error
			}
			done := make(chan outcome, 1)
			go func() { ok, err := st.OrgRenameIdentityOutcome(ctx, id); done <- outcome{ok, err} }()
			select {
			case out := <-done:
				t.Fatal("outcome skipped pending identity transaction", out)
			case <-time.After(100 * time.Millisecond):
			}
			if commit {
				err = tx.Commit(ctx)
			} else {
				err = tx.Rollback(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case out := <-done:
				if out.err != nil || out.committed != commit {
					t.Fatal("wrong authoritative outcome", out)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("outcome did not resume")
			}
		})
	}
}

func TestOrgSlugClaimSurvivesDisposableOrganizationRemoval(t *testing.T) {
	h, _, _ := renameIntentFixture(t)
	ctx := t.Context()
	id := uuid.NewString()
	if _, err := h.Pool.Exec(ctx, `INSERT INTO organizations(id,slug) VALUES($1,'disposable')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, id); err != nil {
		t.Fatal("disposable organization cleanup blocked", err)
	}
	if _, err := h.Pool.Exec(ctx, `INSERT INTO organizations(id,slug) VALUES($1,'disposable')`, uuid.NewString()); err == nil {
		t.Fatal("deleted organization slug was reassigned")
	}
}
