// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// LO QUE SE PRUEBA: que un candidato solo se borra pasado el margen y si ninguna
// fila lo usa, que retirarlo lo salva y que un fallo al borrar el objeto lo deja
// para el siguiente barrido. Es la pieza que impide que una purga de una
// organización borre bytes que otra sigue usando.
func TestBlobsCandidatos(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()

	if err := st.AnotarBlobCandidato(ctx, "shared/drive/aaa"); err != nil {
		t.Fatal(err)
	}
	if err := st.AnotarBlobCandidato(ctx, "shared/drive/aaa"); err != nil {
		t.Fatal("anotar dos veces es idempotente:", err)
	}
	if err := st.AnotarBlobCandidato(ctx, "shared/drive/bbb"); err != nil {
		t.Fatal(err)
	}

	// Dentro del margen no se toca nada.
	var borrados []string
	borrar := func(_ context.Context, key string) error {
		borrados = append(borrados, key)
		return nil
	}
	if n, err := st.PurgarBlobsCandidatos(ctx, time.Now().Add(-time.Hour), 100, borrar); err != nil || n != 0 {
		t.Fatalf("dentro del margen: n=%d err=%v", n, err)
	}
	// Retirar uno (alguien lo reutiliza): sobrevive al barrido.
	if err := st.OlvidarBlobCandidato(ctx, "shared/drive/bbb"); err != nil {
		t.Fatal(err)
	}
	// Pasado el margen, el que queda se borra y desaparece de la lista.
	if n, err := st.PurgarBlobsCandidatos(ctx, time.Now().Add(time.Minute), 100, borrar); err != nil || n != 1 {
		t.Fatalf("pasado el margen: n=%d err=%v", n, err)
	}
	if len(borrados) != 1 || borrados[0] != "shared/drive/aaa" {
		t.Fatalf("borrados = %v", borrados)
	}
	if n, _ := st.PurgarBlobsCandidatos(ctx, time.Now().Add(time.Minute), 100, borrar); n != 0 {
		t.Fatalf("un candidato borrado no vuelve a salir: n=%d", n)
	}

	// Si el objeto no se puede borrar, el candidato se queda para el siguiente barrido.
	_ = st.AnotarBlobCandidato(ctx, "shared/drive/ccc")
	falla := func(context.Context, string) error { return errors.New("s3 caído") }
	if n, err := st.PurgarBlobsCandidatos(ctx, time.Now().Add(time.Minute), 100, falla); err != nil || n != 0 {
		t.Fatalf("con fallo de S3: n=%d err=%v", n, err)
	}
	borrados = nil
	if n, _ := st.PurgarBlobsCandidatos(ctx, time.Now().Add(time.Minute), 100, borrar); n != 1 || borrados[0] != "shared/drive/ccc" {
		t.Fatalf("el candidato debía seguir ahí: n=%d borrados=%v", n, borrados)
	}
}

// Un blob de antes de deduplicar por organización (`shared/drive/<sha256>`) puede
// seguir referenciado desde varias. Que una lo suelte no lo borra: la purga mira
// las filas de TODAS, y solo cuando ninguna lo usa lo entrega para borrar.
func TestLaPurgaNoBorraLoQueOtraOrganizacionUsa(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()
	const clave = "shared/drive/abcdef"
	// Las filas de dos organizaciones se siembran con el rol dueño (sin RLS); la
	// purga corre con el de la aplicación, como en producción.
	admin, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	fichero := map[string]string{}
	for _, org := range []string{"acme", "otra"} {
		var raiz, id string
		if err := admin.QueryRow(ctx, `INSERT INTO folders (tenant, name, is_root) VALUES ($1, '', true) RETURNING id`, org).Scan(&raiz); err != nil {
			t.Fatal(err)
		}
		if err := admin.QueryRow(ctx, `INSERT INTO files (tenant, folder_id, name, storage_key) VALUES ($1, $2, 'mismo.pdf', $3) RETURNING id`, org, raiz, clave).Scan(&id); err != nil {
			t.Fatal(err)
		}
		fichero[org] = id
	}
	var borrados []string
	borrar := func(_ context.Context, key string) error {
		borrados = append(borrados, key)
		return nil
	}
	soltar := func(org string) {
		t.Helper()
		if _, err := admin.Exec(ctx, `DELETE FROM files WHERE id = $1`, fichero[org]); err != nil {
			t.Fatal(err)
		}
		if err := st.AnotarBlobCandidato(ctx, clave); err != nil {
			t.Fatal(err)
		}
	}

	soltar("acme")
	if n, err := st.PurgarBlobsCandidatos(ctx, time.Now().Add(time.Minute), 100, borrar); err != nil || n != 0 || len(borrados) != 0 {
		t.Fatalf("acme lo soltó y otra lo usa: n=%d err=%v borrados=%v; quiero que siga", n, err, borrados)
	}
	soltar("otra")
	if n, err := st.PurgarBlobsCandidatos(ctx, time.Now().Add(time.Minute), 100, borrar); err != nil || n != 1 || len(borrados) != 1 {
		t.Fatalf("ya no lo usa nadie: n=%d err=%v borrados=%v; quiero borrarlo", n, err, borrados)
	}
}
