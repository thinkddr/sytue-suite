// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/almacen"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/transcoder"
)

func TestPrivateCastActualOwnerRowLockDrainsIOAndBlocksLateServiceWorkerAndGC(t *testing.T) {
	f := newPrivateCastFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	releaseWriter := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseWriter)
	done := make(chan error, 1)
	go func() {
		done <- f.st.WithLegacyVideoWrite(f.ctx, "t1", f.video.ID.String()+"/original", func() error { close(started); <-release; return nil })
	}()
	<-started
	admin, err := pgx.Connect(f.ctx, f.h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(f.ctx)
	var writer int32
	if err = admin.QueryRow(f.ctx, `SELECT pid FROM pg_stat_activity WHERE datname=current_database() AND state='idle in transaction' AND query LIKE 'SELECT public.sytue_private_storage_guard(%'`).Scan(&writer); err != nil {
		t.Fatal("actual object writer did not hold retained owner transaction", err)
	}
	locker, err := pgx.Connect(f.ctx, f.h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close(f.ctx)
	tx, err := locker.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	waiting := make(chan error, 1)
	go func() {
		var mode string
		waiting <- tx.QueryRow(f.ctx, `SELECT mode FROM cast_private_storage_states WHERE tenant='t1' AND owner_sub='alice' FOR UPDATE`).Scan(&mode)
	}()
	pgtest.WaitForBlockingPID(f.ctx, t, admin, int32(locker.PgConn().PID()), writer)
	releaseWriter()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-waiting; err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.freeze(t)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	objects, err := objectstore.New(objectstore.Config{Endpoint: strings.TrimPrefix(server.URL, "http://"), Region: "eu-test-1", AccessKey: "synthetic", SecretKey: "synthetic", Bucket: "cast-qa"})
	if err != nil {
		t.Fatal(err)
	}
	protected := almacen.ConFuenteRetenida(objects, f.st)
	var worker transcoder.Objetos = protected
	tenant := tenancy.Tenant{Slug: "t1"}
	prefix := f.video.ID.String() + "/"
	for _, effect := range []func() error{
		func() error {
			return worker.Put(f.ctx, tenant, prefix+"poster.jpg", bytes.NewReader([]byte("new")), 3, objectstore.PutOptions{})
		},
		func() error { return worker.Delete(f.ctx, tenant, prefix+"hls/seg.m4s") },
		func() error { return worker.DeletePrefix(f.ctx, tenant, prefix) },
		func() error {
			return almacen.New(objects, f.st).Put(f.ctx, "t1", prefix+"original", bytes.NewReader([]byte("new")), 3, "video/mp4")
		},
		func() error { return almacen.New(objects, f.st).DeletePrefix(f.ctx, "t1", prefix) },
	} {
		if err := effect(); !privateCastBlocked(err) || calls.Load() != 0 {
			t.Fatal("retained media reached S3 mutation from service/worker/GC", err)
		}
	}
	called := false
	if err := f.st.WithLegacyVideoWrite(f.ctx, "t1", prefix+"original", func() error { called = true; return nil }); !privateCastBlocked(err) || called {
		t.Fatal("delayed ownerless effect escaped actual owner guard", err)
	}
	other := listo(f.ctx, t, f.st, "t1", "bob", domain.VisibilityOrg, domain.StatusReady)
	if err := f.st.WithLegacyVideoWrite(f.ctx, "t1", other.ID.String()+"/original", func() error { called = true; return nil }); err != nil || !called {
		t.Fatal("unrelated legacy owner disabled", err)
	}
	for _, bad := range []string{other.ID.String() + "/../" + f.video.ID.String() + "/original", other.ID.String() + "//original", other.ID.String() + "/\\original"} {
		called = false
		if err := f.st.WithLegacyVideoWrite(f.ctx, "t1", bad, func() error { called = true; return nil }); !errors.Is(err, pg.ErrPrivateStorageScope) || called {
			t.Fatal("ambiguous key reached an external effect", err)
		}
	}
	for _, query := range []string{
		`INSERT INTO person_blocks(blocker_sub,blocked_sub)VALUES('bob','alice')`,
		`INSERT INTO comments(tenant,video_id,author_sub,body)VALUES('t1',$1,'alice','late external contribution')`,
		`UPDATE videos SET owner_sub='bob' WHERE tenant='t1' AND owner_sub='alice'`,
	} {
		err := f.h.PoolSinRLS(t).InTx(f.ctx, func(tx pgx.Tx) error { _, err := tx.Exec(f.ctx, query, queryArgs(query, other.ID)...); return err })
		if !privateCastBlocked(err) {
			t.Fatal("frozen identity escaped OLD/NEW or global relation guard", err)
		}
	}
}

// Query arguments follow these fixed synthetic statements, not user SQL.
func queryArgs(query string, id any) []any {
	if strings.Contains(query, "$1") {
		return []any{id}
	}
	return nil
}

func TestPrivateCastGlobalContributionWaitsForFreezeAndRejectsOldSnapshot(t *testing.T) {
	f := newPrivateCastFixture(t)
	conn, err := pgx.Connect(f.ctx, f.h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(f.ctx)
	freeze, err := conn.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer freeze.Rollback(f.ctx)
	if _, err = freeze.Exec(f.ctx, `SELECT cast_private_subject_freeze('alice')`); err != nil {
		t.Fatal(err)
	}
	writer, err := pgx.Connect(f.ctx, f.h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close(f.ctx)
	result := make(chan error, 1)
	go func() {
		_, err := writer.Exec(f.ctx, `INSERT INTO person_blocks(blocker_sub,blocked_sub)VALUES('bob','alice')`)
		result <- err
	}()
	pgtest.WaitForBlockingPID(f.ctx, t, conn, int32(writer.PgConn().PID()), int32(conn.PgConn().PID()))
	if err := freeze.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal("legacy global contribution failed after lock release", err)
	}
	// A stale repeatable-read writer may not miss a newly created frozen scope.
	old, err := writer.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer old.Rollback(f.ctx)
	_, err = old.Exec(f.ctx, `SELECT 1 FROM person_blocks`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = old.Exec(f.ctx, `INSERT INTO person_blocks(blocker_sub,blocked_sub)VALUES('alice','carol')`)
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != "40001" {
		t.Fatal("global writer accepted stale authority snapshot or failed for unrelated reason", err)
	}
}
