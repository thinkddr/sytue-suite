// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// TestColaHaciaDocs: el disparador de la mig 0027 apunta cada entrada y salida de
// la papelera de un .odt/.ods/.odp por CUALQUIER camino —uno a uno, con su carpeta,
// la auto-purga—, y la cola entrega el estado de AHORA, una fila por fichero.
func TestColaHaciaDocs(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatal(err)
	}
	root, _ := st.EnsureRoot(ctx)
	nuevo := func(carpeta uuid.UUID, nombre string) uuid.UUID {
		t.Helper()
		f, err := st.CreateFile(ctx, store.NewFileRecord{FolderID: carpeta, Name: nombre,
			StorageKey: "files/" + uuid.NewString(), SizeBytes: 10, ContentType: "application/octet-stream", OwnerSub: "ana"})
		if err != nil {
			t.Fatal(err)
		}
		return f.ID
	}
	cola := func() map[uuid.UUID]string {
		t.Helper()
		ps, err := st.PendientesDocs(context.Background(), 0)
		if err != nil {
			t.Fatal(err)
		}
		out := map[uuid.UUID]string{}
		for _, p := range ps {
			if p.OwnerSub != "ana" || p.Tenant != "acme" {
				t.Fatalf("fila inesperada %+v", p)
			}
			out[p.FileID] = p.Estado
		}
		return out
	}

	doc := nuevo(root.ID, "Informe.odt")
	otro := nuevo(root.ID, "foto.jpg")
	if len(cola()) != 0 {
		t.Fatal("crear no encola")
	}
	if _, err := st.DeleteFile(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteFile(ctx, otro); err != nil {
		t.Fatal(err)
	}
	if c := cola(); len(c) != 1 || c[doc] != store.EstadoPapelera {
		t.Fatalf("tras tirar = %v", c)
	}
	// Tirar y restaurar antes de entregar: UNA fila, con el estado de ahora.
	if _, err := st.RestoreFile(ctx, doc); err != nil {
		t.Fatal(err)
	}
	c := cola()
	if len(c) != 1 || c[doc] != store.EstadoVivo {
		t.Fatalf("tras restaurar = %v", c)
	}
	ps, _ := st.PendientesDocs(context.Background(), 0)
	if err := st.QuitarPendientesDocs(context.Background(), ps); err != nil {
		t.Fatal(err)
	}
	if len(cola()) != 0 {
		t.Fatal("lo entregado sigue en la cola")
	}

	// Un cambio que llega DESPUÉS de leer la cola no se pierde al quitar lo leído.
	if _, err := st.DeleteFile(ctx, doc); err != nil {
		t.Fatal(err)
	}
	leidos, _ := st.PendientesDocs(context.Background(), 0)
	time.Sleep(10 * time.Millisecond)
	if _, err := st.RestoreFile(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if err := st.QuitarPendientesDocs(context.Background(), leidos); err != nil {
		t.Fatal(err)
	}
	if c := cola(); c[doc] != store.EstadoVivo {
		t.Fatalf("se perdió el cambio posterior: %v", c)
	}

	// Lo que Docs no acepta se aplaza: no vuelve a salir hasta que toque, y un
	// cambio nuevo lo pone otra vez en cabeza.
	ps, _ = st.PendientesDocs(context.Background(), 0)
	if perdidos, err := st.AplazarPendientesDocs(context.Background(), ps); err != nil || len(perdidos) != 0 {
		t.Fatalf("aplazar: %v, perdidos %v", err, perdidos)
	}
	if c := cola(); len(c) != 0 {
		t.Fatalf("lo aplazado salió antes de tiempo: %v", c)
	}
	if _, err := st.DeleteFile(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if c := cola(); c[doc] != store.EstadoPapelera {
		t.Fatalf("un cambio nuevo no reactivó lo aplazado: %v", c)
	}
	ps, _ = st.PendientesDocs(context.Background(), 0)
	if err := st.QuitarPendientesDocs(context.Background(), ps); err != nil {
		t.Fatal(err)
	}

	// En una unidad de equipo no se encola: Docs no la alcanza en ningún sentido.
	equipo := nuevo(root.ID, "Equipo.odt")
	admin, err := pgx.Connect(context.Background(), h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	for _, q := range []string{
		`UPDATE files SET team_drive_id = gen_random_uuid() WHERE id = $1`,
		`UPDATE files SET deleted_at = now() WHERE id = $1`,
	} {
		if _, err := admin.Exec(context.Background(), q, equipo); err != nil {
			t.Fatal(err)
		}
	}
	if c := cola(); len(c) != 0 {
		t.Fatalf("un .odt de una unidad de equipo entró en la cola: %v", c)
	}

	// Con su carpeta, y purgado por la auto-purga: llega como purgado.
	var carpeta domain.Folder
	carpeta, err = st.CreateFolder(ctx, root.ID, "Sytue Docs", "ana", false)
	if err != nil {
		t.Fatal(err)
	}
	dentro := nuevo(carpeta.ID, "Cuentas.ods")
	if _, err := st.DeleteFolder(ctx, carpeta.ID); err != nil {
		t.Fatal(err)
	}
	if c := cola(); c[dentro] != store.EstadoPapelera {
		t.Fatalf("tirar la carpeta no encoló su .ods: %v", c)
	}
	if _, _, err := st.ClaimAndPurgeExpiredTrash(context.Background(), time.Now().Add(time.Hour), 100); err != nil {
		t.Fatal(err)
	}
	if c := cola(); c[dentro] != store.EstadoPurgado {
		t.Fatalf("la auto-purga no llegó como purgado: %v", c)
	}

	// Otra organización no ve la cola de esta.
	if _, err := st.FicheroEnCualquierEstado(tenantCtx("otra"), doc); err == nil {
		t.Fatal("otra organización ve el fichero")
	}
}
