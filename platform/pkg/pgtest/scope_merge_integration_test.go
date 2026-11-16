// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pgtest_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
)

func scopeMergeFixture(t *testing.T) *pgtest.Harness {
	t.Helper()
	return pgtest.New(t, pg.WithScopeBoundMergeDDL(fstest.MapFS{"migrations/0001_init.up.sql": &fstest.MapFile{Data: []byte(`
-- sytue:scope-bound-merge-v1
CREATE TABLE plain_rows(tenant text NOT NULL,value text NOT NULL);
ALTER TABLE plain_rows ENABLE ROW LEVEL SECURITY;
ALTER TABLE plain_rows FORCE ROW LEVEL SECURITY;
CREATE POLICY plain_merge ON plain_rows USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
CREATE TABLE bound_rows(tenant text NOT NULL,pin text NOT NULL);
ALTER TABLE bound_rows ENABLE ROW LEVEL SECURITY;
ALTER TABLE bound_rows FORCE ROW LEVEL SECURITY;
CREATE POLICY bound_owner ON bound_rows USING(tenant=current_setting('app.tenant',true)) WITH CHECK(tenant=current_setting('app.tenant',true));
-- An earlier permissive merge policy must never override the blocker.
CREATE POLICY bound_old_merge ON bound_rows USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
SELECT sytue_scope_merge_register('bound_rows'::regclass);
`)}}), "migrations")
}

func TestScopeBoundMergeContractDoesNotGrantReadOrRetagging(t *testing.T) {
	h := scopeMergeFixture(t)
	ctx := t.Context()
	admin := h.PoolSinRLS(t)
	if _, err := admin.Exec(ctx, `INSERT INTO bound_rows VALUES('source','retained-original'),('target','existing-target'),('unrelated','foreign')`); err != nil {
		t.Fatal(err)
	}
	inv, err := pg.ConsultaInventarioDeFusion(ctx, h.Pool)
	if err != nil || !slices.Equal(inv.ScopeBound, []string{"bound_rows"}) || !slices.Equal(inv.Movable, []string{"plain_rows"}) || len(inv.Missing) != 0 {
		t.Fatal("incorrect contract inventory", inv, err)
	}
	if err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE bound_rows SET tenant='target' WHERE tenant='source'`)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			t.Fatal("restrictive contract granted merge writes")
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bound_rows`).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("merge gained scope read")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.merge_preflight','on',true),set_config('app.merge_source','source',true),set_config('app.merge_target','target',true)`); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bound_rows`).Scan(&n); err != nil {
			return err
		}
		if n != 2 {
			t.Fatalf("preflight read escaped source/target: %d", n)
		}
		_, err := tx.Exec(ctx, `INSERT INTO bound_rows(tenant,pin) VALUES('source','changed')`)
		var denied *pgconn.PgError
		if !errors.As(err, &denied) || denied.Code != "42501" {
			t.Fatalf("preflight SELECT permitted writes: %v", err)
		}
		return err
	}); err == nil {
		t.Fatal("expected denied write transaction")
	}
}

func TestScopeBoundMergeRejectsBothOrganizationsBeforeAnyDomainWrite(t *testing.T) {
	for _, tenant := range []string{"source", "target"} {
		t.Run(tenant, func(t *testing.T) {
			h := scopeMergeFixture(t)
			admin := h.PoolSinRLS(t)
			if _, err := admin.Exec(t.Context(), `INSERT INTO plain_rows VALUES('source','must-stay')`); err != nil {
				t.Fatal(err)
			}
			if _, err := admin.Exec(t.Context(), `INSERT INTO bound_rows VALUES($1,'fixed-pin')`, tenant); err != nil {
				t.Fatal(err)
			}
			err := h.Pool.InMergeTx(t.Context(), func(tx pgx.Tx) error {
				inv, err := pg.ConsultaInventarioDeFusion(t.Context(), tx)
				if err != nil {
					return err
				}
				if err := pg.CheckScopeBoundMerge(t.Context(), tx, inv, "source", "target"); err != nil {
					return err
				}
				_, err = tx.Exec(t.Context(), `UPDATE plain_rows SET tenant='target' WHERE tenant='source'`)
				return err
			})
			if !errors.Is(err, pg.ErrScopeBoundMerge) {
				t.Fatal("scope-bound data did not block before writes", err)
			}
			var n int
			if err := admin.QueryRow(context.Background(), `SELECT count(*) FROM plain_rows WHERE tenant='source' AND value='must-stay'`).Scan(&n); err != nil || n != 1 {
				t.Fatal("preflight moved domain rows", n, err)
			}
			if err := admin.QueryRow(context.Background(), `SELECT count(*) FROM bound_rows WHERE tenant=$1 AND pin='fixed-pin'`, tenant).Scan(&n); err != nil || n != 1 {
				t.Fatal("preflight rewrote scope or pin", n, err)
			}
		})
	}
}

func TestScopeBoundMergeMissingGuardIsNotConvertedToMovable(t *testing.T) {
	for _, part := range []string{"scope_bound_merge_write", "scope_bound_merge_read"} {
		t.Run(part, func(t *testing.T) {
			h := scopeMergeFixture(t)
			admin := h.PoolSinRLS(t)
			if _, err := admin.Exec(t.Context(), `DROP POLICY `+pgx.Identifier{part}.Sanitize()+` ON bound_rows`); err != nil {
				t.Fatal(err)
			}
			inv, err := pg.ConsultaInventarioDeFusion(t.Context(), h.Pool)
			if err != nil || !slices.Equal(inv.Missing, []string{"bound_rows"}) || slices.Contains(inv.Movable, "bound_rows") {
				t.Fatal("incomplete blocker became a merge permission", inv, err)
			}
		})
	}
}

func TestScopeBoundMergeRollbackCannotRemoveProtectionForRetainedData(t *testing.T) {
	h := scopeMergeFixture(t)
	admin := h.PoolSinRLS(t)
	if _, err := admin.Exec(t.Context(), `INSERT INTO bound_rows VALUES('unrelated','retained-pin')`); err != nil {
		t.Fatal(err)
	}
	_, err := admin.Exec(t.Context(), `SELECT sytue_scope_merge_unregister(ARRAY['bound_rows'::regclass])`)
	var blocked *pgconn.PgError
	if !errors.As(err, &blocked) || blocked.Code != "PST01" {
		t.Fatal("rollback removed a retained Scope contract", err)
	}
	inventory, err := pg.ConsultaInventarioDeFusion(t.Context(), h.Pool)
	if err != nil || !slices.Equal(inventory.ScopeBound, []string{"bound_rows"}) || len(inventory.Missing) != 0 {
		t.Fatal("failed rollback changed the contract", inventory, err)
	}
}
