//go:build integration

// SPDX-License-Identifier: Apache-2.0
package pgtest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

func TestSessionOwnershipCannotBorrowReplacementAfterBackendDeath(t *testing.T) {
	h := scopeMergeFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	first, err := pg.Connect(ctx, pg.Config{DSN: h.AppDSN, DirectDSN: h.AppDSN, MaxConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := pg.Connect(ctx, pg.Config{DSN: h.AppDSN, DirectDSN: h.AppDSN, MaxConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	key := "synthetic-session:" + uuid.NewString()
	owner, err := pg.TrySessionLock(ctx, first, key)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close(ctx) }()
	if _, err := pg.TrySessionLock(ctx, second, key); !errors.Is(err, pg.ErrSessionLockBusy) {
		t.Fatal("competing owner admitted", err)
	}
	var pid int32
	if err := owner.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := owner.InMergeTx(ctx, func(tx pgx.Tx) error {
			var actual int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&actual); err != nil {
				return err
			}
			if actual != pid {
				return errors.New("transaction switched physical owner")
			}
			_, err := tx.Exec(ctx, `INSERT INTO plain_rows VALUES('source','owned')`)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.PoolSinRLS(t).Exec(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatal(err)
	}
	err = owner.InMergeTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO plain_rows VALUES('source','zombie replacement')`)
		return err
	})
	if err == nil {
		t.Fatal("dead owner reacquired a connection and published")
	}
	var n int
	if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM plain_rows WHERE value='zombie replacement'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("zombie owner committed a replacement write", n, err)
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	var replacement *pg.Session
	for {
		replacement, err = pg.TrySessionLock(ctx, second, key)
		if err == nil {
			break
		}
		if !errors.Is(err, pg.ErrSessionLockBusy) {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatal("actual dead session retained lock", ctx.Err())
		case <-ticker.C:
		}
	}
	if err := replacement.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `INSERT INTO plain_rows VALUES('source','after close')`); err == nil {
		t.Fatal("closed session emitted a write")
	}
}
