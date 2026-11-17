// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

func TestPrivatePhotosInventoryRetainsTrashRecipesVectorsAndShares(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := ctxDe("t1")
	asset, err := st.Ingerir(ctx, foto("original.heic", "retained-drive-original", nil))
	if err != nil {
		t.Fatal(err)
	}
	album, err := st.CrearAlbum(ctx, "u1", "retained album")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AnyadirAlAlbum(ctx, album.ID, "u1", []uuid.UUID{asset.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GuardarReceta(ctx, asset.ID, "u1", domain.RecetaVacia()); err != nil {
		t.Fatal(err)
	}
	if err := st.ALaPapelera(ctx, asset.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "t1", Subject: "u1", Product: "photos"}
	read := func(ctx context.Context, scope storage.Scope) (*model.Snapshot, error) {
		var snapshot *model.Snapshot
		err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			var err error
			snapshot, err = store.ReadPrivateStorageInventory(ctx, tx, scope)
			return err
		})
		return snapshot, err
	}
	snapshot, err := read(ctx, scope)
	if err != nil || snapshot.PersonalCompatible() != nil {
		t.Fatal("personal inventory", err)
	}
	if len(snapshot.Rows()) != 4 || len(snapshot.Originals()) != 1 {
		t.Fatal("retained row omitted")
	}
	var retainedAsset []byte
	for _, row := range snapshot.Rows() {
		if row.Kind == "asset" {
			retainedAsset = row.JSON
		}
	}
	if !bytes.Contains(retainedAsset, []byte(`"deleted_at":`)) || !bytes.Contains(retainedAsset, []byte(`"embedding_model":`)) || !bytes.Contains(retainedAsset, []byte(`"drive_file_id": "retained-drive-original"`)) {
		t.Fatal("source projection lost columns")
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO album_personas(album_id,tenant,sujeto,concedido_por,puede_editar) VALUES($1,'t1','u2','u1',true)`, album.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"u1", "u2"} {
		scope.Subject = subject
		snapshot, err = read(ctx, scope)
		if err != nil || len(snapshot.Rows()) != 5 || !errors.Is(snapshot.PersonalCompatible(), model.ErrShared) {
			t.Fatal("incoming/outgoing ACL lost", subject, err)
		}
	}
	scope.Subject = "unrelated"
	snapshot, err = read(ctx, scope)
	if err != nil || len(snapshot.Rows()) != 0 {
		t.Fatal("unrelated person's photos leaked", err)
	}
	scope.Tenant = "other"
	if _, err = read(ctx, scope); !errors.Is(err, pg.ErrPrivateStorageScope) {
		t.Fatal("scope rebound", err)
	}
}
