// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// «Soltar la fuente» de Sytue One sobre su esquema real: solo la persona, solo la
// época congelada, y la fuente vuelve a legacy y se puede congelar otra vez.
func TestSoltarLaFuenteDeOne(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	scope := storage.Scope{Tenant: "acme", Subject: "ana", Product: "hub"}
	epoch := uuid.New()
	// hub_freeze_private_storage lee una tabla que no existe (notification_preferences,
	// 0011): Begin falla hoy en One. Se congela a mano, como lo dejaría una captura.
	admin, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	if _, err := admin.Exec(ctx, `INSERT INTO hub_private_storage_states(tenant,owner_sub,mode,epoch,started_at) VALUES('acme','ana','frozen',$1,now())`, epoch); err != nil {
		t.Fatal(err)
	}
	release := pg.PrivateStorageRelease{Epoch: uuid.New(), AuthTime: time.Now(), AMR: []string{"pwd"}}
	if err := st.ReleasePrivateStorage(ctx, scope, release); !errors.Is(err, pg.ErrPrivateStorageReleaseStale) {
		t.Fatal("soltó otra época", err)
	}
	other := scope
	other.Product = "notes"
	release.Epoch = epoch
	if err := st.ReleasePrivateStorage(ctx, other, release); err == nil {
		t.Fatal("soltó con el ámbito de otro producto")
	}
	if err := st.ReleasePrivateStorage(ctx, scope, release); err != nil {
		t.Fatal(err)
	}
	if state, err := st.PrivateStorageState(ctx, "ana"); err != nil || state.Mode != "legacy" {
		t.Fatal("no volvió a legacy", state, err)
	}
	if _, err := admin.Exec(ctx, `UPDATE hub_private_storage_states SET mode='frozen',epoch=$1,started_at=now() WHERE tenant='acme' AND owner_sub='ana'`, uuid.New()); err != nil {
		t.Fatal("una fuente soltada no se puede volver a congelar", err)
	}
}
