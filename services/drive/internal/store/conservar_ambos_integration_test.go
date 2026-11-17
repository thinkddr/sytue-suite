// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// TestConservarAmbos: subir «IMG_0001.jpg» a una carpeta que ya lo tiene SUSTITUYE (una
// versión nueva del mismo fichero), salvo con store.ConservarAmbos, que deja el de antes
// intacto y guarda el nuevo como «IMG_0001 (2).jpg», y el siguiente como «(3)». La
// comparación de nombres es la de siempre, sin distinguir mayúsculas.
func TestConservarAmbos(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<20); err != nil {
		t.Fatal(err)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	primera := seedFile(t, ctx, st, root.ID, "IMG_0001.jpg", 10)

	ambos := store.ConservarAmbos(ctx)
	segunda, err := st.CreateFile(ambos, store.NewFileRecord{
		FolderID: root.ID, Name: "img_0001.JPG", StorageKey: "files/b", SizeBytes: 20, ContentType: "image/jpeg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if segunda.ID == primera.ID || segunda.Name != "img_0001 (2).JPG" || segunda.Version != 1 {
		t.Fatalf("con ConservarAmbos: %+v; quiero un fichero NUEVO «img_0001 (2).JPG»", segunda)
	}
	tercera, err := st.CreateFile(ambos, store.NewFileRecord{
		FolderID: root.ID, Name: "IMG_0001.jpg", StorageKey: "files/c", SizeBytes: 30, ContentType: "image/jpeg",
	})
	if err != nil || tercera.Name != "IMG_0001 (3).jpg" {
		t.Fatalf("la tercera: %+v, %v", tercera, err)
	}
	antes, err := st.GetFile(ctx, primera.ID)
	if err != nil || antes.SizeBytes != 10 || antes.Version != 1 {
		t.Fatalf("la primera ha cambiado: %+v, %v", antes, err)
	}
	q, err := st.GetQuota(ctx)
	if err != nil || q.UsedBytes != 60 {
		t.Fatalf("cuota %+v, %v; quiero 60 bytes (10+20+30)", q, err)
	}

	// Sin la opción, lo de siempre: versión nueva del mismo fichero.
	sustituida, err := st.CreateFile(ctx, store.NewFileRecord{
		FolderID: root.ID, Name: "IMG_0001.jpg", StorageKey: "files/d", SizeBytes: 40, ContentType: "image/jpeg",
	})
	if err != nil || sustituida.ID != primera.ID || sustituida.Version != 2 {
		t.Fatalf("sin ConservarAmbos: %+v, %v; quiero la versión 2 de la primera", sustituida, err)
	}
}
