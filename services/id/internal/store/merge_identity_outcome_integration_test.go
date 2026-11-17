// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func TestMergeIdentityOutcomeWaitsForCommitOrRollback(t *testing.T) {
	for _, commit := range []bool{false, true} {
		name := "rollback"
		if commit {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
			h := pgtest.New(t, fstest.MapFS{"migrations/0001_init.up.sql": &fstest.MapFile{Data: []byte(`CREATE TABLE organizations(id uuid PRIMARY KEY,status text NOT NULL DEFAULT 'active',merged_into_org_id uuid);`)}}, "migrations")
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			source, target := uuid.NewString(), uuid.NewString()
			if _, err := h.Pool.Exec(ctx, `INSERT INTO organizations(id) VALUES($1),($2)`, source, target); err != nil {
				t.Fatal(err)
			}
			writer, err := h.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = writer.Rollback(ctx) }()
			var writerPID int32
			if err := writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Exec(ctx, `UPDATE organizations SET status='merged',merged_into_org_id=$2 WHERE id=$1`, source, target); err != nil {
				t.Fatal(err)
			}
			reader, err := pg.Connect(ctx, pg.Config{DSN: h.AppDSN, MaxConns: 1})
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			var readerPID int32
			if err := reader.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&readerPID); err != nil {
				t.Fatal(err)
			}
			type outcome struct {
				committed bool
				err       error
			}
			done := make(chan outcome, 1)
			go func() {
				committed, err := store.New(reader).MergeIdentityOutcome(ctx, source, target)
				done <- outcome{committed, err}
			}()
			pgtest.WaitForBlockingPID(ctx, t, h.Pool, readerPID, writerPID)
			if commit {
				err = writer.Commit(ctx)
			} else {
				err = writer.Rollback(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			result := <-done
			if result.err != nil || result.committed != commit {
				t.Fatal("identity outcome did not settle before admission decision", result, commit)
			}
		})
	}
}
