// SPDX-License-Identifier: Apache-2.0
package pgtest

import (
	"context"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// WaitBlocked observes PostgreSQL's real lock graph; it does not infer
// serialization from sleeping or from a goroutine that may not have run yet.
func WaitBlocked(ctx context.Context, t testing.TB, pool pg.Querier, blocked, blocker int32) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT $1=ANY(pg_blocking_pids($2))`, blocker, blocked).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("expected PostgreSQL lock was not observed", ctx.Err())
		}
	}
}
