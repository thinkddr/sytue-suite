// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

type rollbackVariants struct {
	key     string
	pause   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (v *rollbackVariants) List(ctx context.Context, _ tenancy.Tenant, _, _ string, _ int) (objectstore.ObjectPage, error) {
	if v.pause.Load() {
		select {
		case v.entered <- struct{}{}:
		default:
		}
		select {
		case <-v.release:
		case <-ctx.Done():
			return objectstore.ObjectPage{}, ctx.Err()
		}
	}
	return objectstore.ObjectPage{Objects: []objectstore.ListedObject{{Key: v.key, Size: 1}}}, nil
}

func (*rollbackVariants) Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte{1})), nil
}

func rollbackSourceFixture(t *testing.T) (*PrivateSource, *pgtest.Harness, *rollbackVariants, storage.Scope, context.Context) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	ctx, cancel := context.WithTimeout(tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant}), 20*time.Second)
	t.Cleanup(cancel)
	st := New(h.Pool)
	asset, err := st.Ingerir(ctx, domain.Activo{Duenyo: scope.Subject, DriveID: uuid.NewString(), Nombre: "retained.png", Tipo: "image/png", Bytes: 1, Ancho: 16, Alto: 12, Orientacion: 1})
	if err != nil {
		t.Fatal(err)
	}
	key, err := (model.Derivada{ActivoID: asset.ID, Huella: "retained", Variante: model.VarianteMiniatura, Formato: "png"}).Clave()
	if err != nil {
		t.Fatal(err)
	}
	variants := &rollbackVariants{key: key, entered: make(chan struct{}, 1), release: make(chan struct{})}
	return &PrivateSource{store: st, objects: variants}, h, variants, scope, ctx
}

func rollbackAdmin(ctx context.Context, t *testing.T, h *pgtest.Harness) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func waitPreflightRelationLock(ctx context.Context, t *testing.T, observer *pgx.Conn, pid int32, mode string, done <-chan error) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := observer.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE ($1::int=0 OR pid=$1) AND mode=$2 AND NOT granted AND relation IN('photos_private_storage_preflights'::regclass,'photos_private_storage_preflight_inventory'::regclass))`, pid, mode).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case err := <-done:
			t.Fatal("operation completed before reaching the expected relation lock", err)
		case <-ctx.Done():
			t.Fatal("operation never reached the expected relation lock")
		case <-ticker.C:
		}
	}
}

func TestPrivatePhotosRollbackWaitsForAcceptAndRetainsCommittedPlanInventory(t *testing.T) {
	source, h, variants, scope, ctx := rollbackSourceFixture(t)
	epoch := uuid.NewString()
	info, err := source.StartPreflight(ctx, scope, epoch)
	if err != nil {
		t.Fatal(err)
	}
	down, err := fs.ReadFile(db.Migrations, "migrations/0016_private_preflight_inventory.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	admin := rollbackAdmin(ctx, t, h)
	observer := rollbackAdmin(ctx, t, h)
	var pid int32
	if err := admin.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	variants.pause.Store(true)
	release := sync.OnceFunc(func() { close(variants.release) })
	t.Cleanup(release)
	accepted := make(chan error, 1)
	link := storage.Reference{RoomID: "!photos:test", EventID: "$accepted"}
	go func() { _, err := source.AcceptPlan(ctx, scope, epoch, info.CAS, link); accepted <- err }()
	select {
	case <-variants.entered:
	case <-ctx.Done():
		t.Fatal("Accept did not reach its fenced external capture")
	}
	rolledBack := make(chan error, 1)
	go func() {
		tx, err := admin.Begin(ctx)
		if err != nil {
			rolledBack <- err
			return
		}
		_, err = tx.Exec(ctx, string(down))
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		rolledBack <- err
	}()
	waitPreflightRelationLock(ctx, t, observer, pid, "AccessExclusiveLock", rolledBack)
	release()
	if err := <-accepted; err != nil {
		t.Fatal("Accept could not confirm ahead of rollback", err)
	}
	var failure *pgconn.PgError
	if err := <-rolledBack; !errors.As(err, &failure) || failure.Code != "PST01" {
		t.Fatal("rollback removed proof of a concurrently confirmed plan", err)
	}
	var count int64
	if err := observer.QueryRow(ctx, `SELECT count(*) FROM photos_private_storage_preflight_inventory WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3`, scope.Tenant, scope.Subject, epoch).Scan(&count); err != nil || count != info.Rows {
		t.Fatal("committed inventory proof disappeared", count, err)
	}
	routing, err := source.PlanRouting(ctx, scope)
	if err != nil || routing.Plan == nil || routing.Plan.Link != link {
		t.Fatal("concurrent plan confirmation disappeared", err)
	}
}

func TestPrivatePhotosAcceptCannotConfirmAfterRollbackWins(t *testing.T) {
	source, h, _, scope, ctx := rollbackSourceFixture(t)
	epoch := uuid.NewString()
	info, err := source.StartPreflight(ctx, scope, epoch)
	if err != nil {
		t.Fatal(err)
	}
	admin := rollbackAdmin(ctx, t, h)
	observer := rollbackAdmin(ctx, t, h)
	down, err := fs.ReadFile(db.Migrations, "migrations/0016_private_preflight_inventory.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(down)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	accepted := make(chan error, 1)
	go func() {
		_, err := source.AcceptPlan(ctx, scope, epoch, info.CAS, storage.Reference{RoomID: "!photos:test", EventID: "$late"})
		accepted <- err
	}()
	waitPreflightRelationLock(ctx, t, observer, 0, "RowExclusiveLock", accepted)
	// The committed downgrade serializes before this Accept. The missing
	// inventory table must make the transaction abort, never a new accepted plan.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-accepted; err == nil {
		t.Fatal("Accept confirmed after its required inventory had been removed")
	}
	var confirmed bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM photos_private_storage_preflights WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND plan_event_id IS NOT NULL)`, scope.Tenant, scope.Subject, epoch).Scan(&confirmed); err != nil || confirmed {
		t.Fatal("rollback-first race left an unprovable confirmed plan", err)
	}
}
