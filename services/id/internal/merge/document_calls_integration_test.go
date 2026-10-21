// SPDX-License-Identifier: Apache-2.0
//go:build integration

package merge

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
)

type mergeCallRevoker struct {
	err      error
	versions []int64
}

func (f *mergeCallRevoker) RevokeDocumentCalls(_ context.Context, _, _ string, version int64) error {
	f.versions = append(f.versions, version)
	return f.err
}

func TestDocsOrganizationMoveRevokesCallsBeforeCommitAndRollsBackFailures(t *testing.T) {
	h := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	ctx := context.Background()
	const id = "2f88dc9e-0396-4df1-9728-72cc7209ac64"
	if err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO documents(id,tenant,kind,title,owner_sub) VALUES($1,'source-org','document','Private','owner')`, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE doc_call_guard_state SET enabled=true WHERE singleton`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	revoker := &mergeCallRevoker{err: errors.New("revocation unavailable")}
	if _, err := MoveService(ctx, h.Pool, "source-org", "target-org", revoker); err == nil {
		t.Fatal("organization move committed before revocation")
	}
	revoker.err = nil
	if _, err := MoveService(ctx, h.Pool, "source-org", "target-org", revoker); err != nil {
		t.Fatal(err)
	}
	if len(revoker.versions) != 2 || revoker.versions[1] <= revoker.versions[0] {
		t.Fatalf("rollback reused access version: %v", revoker.versions)
	}
	if err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		var tenant string
		if err := tx.QueryRow(ctx, `SELECT tenant FROM documents WHERE id=$1`, id).Scan(&tenant); err != nil {
			return err
		}
		if tenant != "target-org" {
			t.Fatalf("document not moved: %s", tenant)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
