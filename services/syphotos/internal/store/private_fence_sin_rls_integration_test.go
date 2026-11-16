// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	photosDrive "git.sytue.eu/sytue/one/services/syphotos/internal/drive"
	"git.sytue.eu/sytue/one/services/syphotos/internal/service"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

func privatePhotoBlocked(err error) bool {
	var failure *pgconn.PgError
	return errors.As(err, &failure) && failure.Code == "PST01"
}

// This helper captures SQL for focused fence tests. It does not mount a product
// migration capability or claim complete media import; production capture also
// requires the immutable client plan and full variant inventory.
func freezePhotoMetadata(ctx context.Context, h *pgtest.Harness, owner string, check func()) error {
	return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, owner); err != nil {
			return err
		}
		scope := storage.Scope{Tenant: "t1", Subject: owner, Product: "photos"}
		_, err := pg.BeginPrivateStorageSnapshot(ctx, tx, pg.PrivateSnapshotConfig{Scope: scope, Epoch: uuid.New(), Capture: func(ctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
			if check != nil {
				check()
			}
			snapshot, err := store.ReadPrivateStorageInventory(ctx, tx, scope)
			if err != nil {
				return pg.PrivateSnapshotCounts{}, err
			}
			for _, row := range snapshot.Rows() {
				if _, err := tx.Exec(ctx, `INSERT INTO photos_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload,size_bytes,sha256)SELECT $1,$2,epoch,$3,$4,$5,octet_length($5::bytea),encode(digest($5::bytea,'sha256'),'hex') FROM photos_private_storage_states WHERE tenant=$1 AND owner_sub=$2`, scope.Tenant, owner, row.Kind, row.ID, row.JSON); err != nil {
					return pg.PrivateSnapshotCounts{}, err
				}
			}
			return pg.PrivateSnapshotCounts{Rows: int64(len(snapshot.Rows()))}, nil
		}})
		return err
	})
}

func TestPrivatePhotoFenceWaitsForExternalWriterAndStopsRealEncoderAndCleaner(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := ctxDe("t1")
	asset, err := st.Ingerir(ctx, foto("retained.png", "private-fence-original", nil))
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	releaseWriter := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseWriter)
	writerDone := make(chan error, 1)
	var complete atomic.Bool
	go func() {
		writerDone <- st.WithLegacyOwnerWrite(ctx, "u1", func() error { close(started); <-release; complete.Store(true); return nil })
	}()
	<-started
	frozen := make(chan error, 1)
	go func() {
		frozen <- freezePhotoMetadata(ctx, h, "u1", func() {
			if !complete.Load() {
				t.Error("capture ran before complete external write")
			}
		})
	}()
	select {
	case err := <-frozen:
		t.Fatal("freeze escaped external writer lock", err)
	case <-time.After(100 * time.Millisecond):
	}
	releaseWriter()
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-frozen; err != nil {
		t.Fatal(err)
	}
	var ioCalls atomic.Int64
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ioCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(external.Close)
	objects, err := objectstore.New(objectstore.Config{Endpoint: strings.TrimPrefix(external.URL, "http://"), Region: "eu-test-1", AccessKey: "synthetic", SecretKey: "synthetic", Bucket: "retained-photos"})
	if err != nil {
		t.Fatal(err)
	}
	driver := photosDrive.Nuevo(external.URL, time.Second)
	encoder := service.NuevaIngesta(st, driver, objects, 0, nil)
	if err := encoder.Derivar(ctx, "synthetic-token", asset, domain.RecetaVacia()); !privatePhotoBlocked(err) || ioCalls.Load() != 0 {
		t.Fatal("frozen owner reached server decode/Drive/S3", err)
	}
	cleaner := service.NuevaLimpieza(objects, st)
	if failures := cleaner.BorrarDerivadas(ctx, "t1", "u1", asset.ID, ""); failures == 0 || ioCalls.Load() != 0 {
		t.Fatal("frozen owner's delayed cleaner reached S3", failures)
	}
	if failures := cleaner.BorrarDerivadas(ctx, "t1", "", asset.ID, ""); failures == 0 || ioCalls.Load() != 0 {
		t.Fatal("ownerless cleaner reached S3", failures)
	}
	if err := st.WithLegacyOwnerWrite(ctx, "u1", func() error { ioCalls.Add(1); return nil }); !privatePhotoBlocked(err) || ioCalls.Load() != 0 {
		t.Fatal("frozen external writer started", err)
	}
	if err := st.WithLegacyOwnerWrite(ctx, "unrelated", func() error { ioCalls.Add(1); return nil }); err != nil || ioCalls.Load() != 1 {
		t.Fatal("unrelated legacy owner blocked", err)
	}
}

func TestPrivatePhotoFenceCoversOldNewParentsAndIncomingACLUnderMaintenance(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := ctxDe("t1")
	privileged := h.PoolSinRLS(t)
	asset, err := st.Ingerir(ctx, foto("retained.png", "private-fence-resource", nil))
	if err != nil {
		t.Fatal(err)
	}
	alice, err := st.CrearAlbum(ctx, "u1", "Retained")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := st.CrearAlbum(ctx, "u2", "Legacy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AnyadirAlAlbum(ctx, alice.ID, "u1", []uuid.UUID{asset.ID}); err != nil {
		t.Fatal(err)
	}
	if err := freezePhotoMetadata(ctx, h, "u1", nil); err != nil {
		t.Fatal(err)
	}
	queries := map[string]struct {
		sql  string
		args []any
	}{
		"asset update":       {`UPDATE assets SET description='changed' WHERE id=$1`, []any{asset.ID}},
		"asset delete":       {`DELETE FROM assets WHERE id=$1`, []any{asset.ID}},
		"owner rename":       {`UPDATE assets SET owner_sub='u2' WHERE id=$1`, []any{asset.ID}},
		"album update":       {`UPDATE albums SET title='changed' WHERE id=$1`, []any{alice.ID}},
		"cover cross owner":  {`UPDATE albums SET cover_asset_id=$1 WHERE id=$2`, []any{asset.ID, bob.ID}},
		"member insert":      {`INSERT INTO album_assets(tenant,album_id,asset_id)VALUES('t1',$1,$2)`, []any{bob.ID, asset.ID}},
		"member move":        {`UPDATE album_assets SET album_id=$1 WHERE album_id=$2`, []any{bob.ID, alice.ID}},
		"edit foreign owner": {`INSERT INTO edits(tenant,asset_id,owner_sub,recipe,fingerprint)VALUES('t1',$1,'u2','{}','retained')`, []any{asset.ID}},
		"outgoing ACL":       {`INSERT INTO album_personas(tenant,album_id,sujeto,concedido_por)VALUES('t1',$1,'u2','u1')`, []any{alice.ID}},
		"incoming ACL":       {`INSERT INTO album_personas(tenant,album_id,sujeto,concedido_por)VALUES('t1',$1,'u1','u2')`, []any{bob.ID}},
		"source delete":      {`DELETE FROM photos_private_storage_snapshot WHERE owner_sub='u1'`, nil},
		"state reopen":       {`UPDATE photos_private_storage_states SET mode='legacy',epoch=NULL,started_at=NULL WHERE owner_sub='u1'`, nil},
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			err := privileged.InMaintenanceTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, query.sql, query.args...); return err })
			if !privatePhotoBlocked(err) {
				t.Fatal("frozen relationship or retained source changed", err)
			}
		})
	}
}
