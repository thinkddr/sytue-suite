// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pgtest

import (
	"context"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// WaitForBlockingPID observes PostgreSQL's actual lock dependency. A delay or a
// goroutine merely starting is not evidence that a concurrency guard blocked.
func WaitForBlockingPID(ctx context.Context, t *testing.T, q pg.Querier, blocked, blocker int32) {
	t.Helper()
	timer := time.NewTicker(5 * time.Millisecond)
	defer timer.Stop()
	for {
		var waiting bool
		if err := q.QueryRow(ctx, `SELECT $1::integer=ANY(pg_blocking_pids($2::integer))`, blocker, blocked).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("expected PostgreSQL lock dependency", ctx.Err())
		case <-timer.C:
		}
	}
}
