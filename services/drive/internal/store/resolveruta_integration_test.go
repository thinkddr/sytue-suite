// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Resolución de rutas (ResolveUnder) contra un PostgreSQL real.
//
// Es el ÚNICO sitio donde esto se puede comprobar: el descenso lo hace un
// WITH RECURSIVE, así que un doble en memoria no prueba nada del SQL —solo que
// el doble se parece a lo que uno cree que hace el SQL—. Lo que se vigila:
//
//   - una ruta profunda resuelve carpeta y fichero,
//   - las mayúsculas no importan, igual que con strings.EqualFold, que es lo que
//     hacía la versión en Go (y lo que ya impone folders_unique_sibling_name),
//   - un tramo intermedio inexistente NO resuelve,
//   - un FICHERO no es un tramo intermedio válido: /a/b.txt/c no existe,
//   - lo borrado (deleted_at) no resuelve, ni de tramo ni de hoja,
//   - con el mismo nombre en carpeta y fichero gana el fichero, que es el orden
//     que tenía la resolución en Go.
package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// arbolProfundo monta /a/b/c con d.pdf dentro de c y devuelve los ids útiles.
func arbolProfundo(t *testing.T, ctx context.Context, st *store.Store) (root, c uuid.UUID) {
	t.Helper()
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	raiz, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	actual := raiz.ID
	for _, nombre := range []string{"a", "b", "c"} {
		f, err := st.CreateFolder(ctx, actual, nombre, "sub", false)
		if err != nil {
			t.Fatalf("CreateFolder(%q): %v", nombre, err)
		}
		actual = f.ID
	}
	seedFile(t, ctx, st, actual, "d.pdf", 10)
	return raiz.ID, actual
}

func TestResolveUnderRutaProfunda(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	root, _ := arbolProfundo(t, ctx, st)

	// El fichero del final.
	carpeta, fichero, err := st.ResolveUnder(ctx, root, []string{"a", "b", "c", "d.pdf"})
	if err != nil {
		t.Fatalf("ResolveUnder fichero: %v", err)
	}
	if fichero == nil || fichero.Name != "d.pdf" || carpeta != nil {
		t.Fatalf("esperaba el fichero d.pdf, obtuve carpeta=%+v fichero=%+v", carpeta, fichero)
	}

	// Una carpeta intermedia como destino.
	carpeta, fichero, err = st.ResolveUnder(ctx, root, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("ResolveUnder carpeta: %v", err)
	}
	if carpeta == nil || carpeta.Name != "c" || fichero != nil {
		t.Fatalf("esperaba la carpeta c, obtuve carpeta=%+v fichero=%+v", carpeta, fichero)
	}

	// Un solo tramo, que es el caso de borde del descenso: el padre es la raíz y
	// la recursión no da ni un paso.
	carpeta, _, err = st.ResolveUnder(ctx, root, []string{"a"})
	if err != nil {
		t.Fatalf("ResolveUnder un tramo: %v", err)
	}
	if carpeta == nil || carpeta.Name != "a" {
		t.Fatalf("esperaba la carpeta a, obtuve %+v", carpeta)
	}
}

func TestResolveUnderIgnoraMayusculas(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	root, _ := arbolProfundo(t, ctx, st)

	// Es la misma regla que ya impone `folders_unique_sibling_name` sobre
	// lower(name), y la que aplicaba `strings.EqualFold` en la versión en Go.
	_, fichero, err := st.ResolveUnder(ctx, root, []string{"A", "B", "C", "D.PDF"})
	if err != nil {
		t.Fatalf("ResolveUnder: %v", err)
	}
	if fichero == nil || fichero.Name != "d.pdf" {
		t.Fatalf("esperaba d.pdf resolviendo en mayúsculas, obtuve %+v", fichero)
	}
}

func TestResolveUnderNoResuelveLoQueNoEsRuta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	root, _ := arbolProfundo(t, ctx, st)

	casos := map[string][]string{
		"tramo intermedio inexistente": {"a", "b", "no-existe", "d.pdf"},
		"hoja inexistente":             {"a", "b", "c", "no-existe.pdf"},
		// Un fichero no se puede atravesar: es hoja por definición.
		"atravesar un fichero": {"a", "b", "c", "d.pdf", "x"},
	}
	for nombre, segs := range casos {
		carpeta, fichero, err := st.ResolveUnder(ctx, root, segs)
		if err != nil {
			t.Fatalf("%s: ResolveUnder: %v", nombre, err)
		}
		if carpeta != nil || fichero != nil {
			t.Fatalf("%s: no debía resolver, obtuve carpeta=%+v fichero=%+v", nombre, carpeta, fichero)
		}
	}
}

func TestResolveUnderNoVeLoBorrado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	root, c := arbolProfundo(t, ctx, st)

	// A la papelera el fichero hoja.
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE files SET deleted_at = now() WHERE folder_id = $1`, c)
		return err
	}); err != nil {
		t.Fatalf("borrar el fichero: %v", err)
	}
	if _, fichero, err := st.ResolveUnder(ctx, root, []string{"a", "b", "c", "d.pdf"}); err != nil || fichero != nil {
		t.Fatalf("un fichero en la papelera no debe resolver: %+v (err=%v)", fichero, err)
	}

	// Y una carpeta INTERMEDIA borrada corta la ruta entera, no solo su tramo.
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE folders SET deleted_at = now() WHERE name = 'b'`)
		return err
	}); err != nil {
		t.Fatalf("borrar la carpeta b: %v", err)
	}
	carpeta, _, err := st.ResolveUnder(ctx, root, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("ResolveUnder: %v", err)
	}
	if carpeta != nil {
		t.Fatalf("una carpeta intermedia borrada debe cortar la ruta, obtuve %+v", carpeta)
	}
}

// Carpeta y fichero pueden llamarse igual dentro del mismo padre: la unicidad
// de nombre es por tabla, no entre las dos. Cuando pasa, ResolveUnder devuelve
// LOS DOS y no elige: quién gana lo decide cada llamante, porque no coinciden
// —WebDAV prefiere el fichero y un enlace público la carpeta—. Decidirlo aquí
// cambiaría en silencio una de las dos.
func TestResolveUnderDevuelveAmbosSiSeLlamanIgual(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	root, c := arbolProfundo(t, ctx, st)

	if _, err := st.CreateFolder(ctx, c, "d.pdf", "sub", false); err != nil {
		t.Fatalf("CreateFolder homónima: %v", err)
	}
	carpeta, fichero, err := st.ResolveUnder(ctx, root, []string{"a", "b", "c", "d.pdf"})
	if err != nil {
		t.Fatalf("ResolveUnder: %v", err)
	}
	if fichero == nil || carpeta == nil {
		t.Fatalf("con nombres iguales tienen que venir los dos, obtuve carpeta=%+v fichero=%+v", carpeta, fichero)
	}
	if fichero.Name != "d.pdf" || carpeta.Name != "d.pdf" {
		t.Fatalf("nombres inesperados: carpeta=%q fichero=%q", carpeta.Name, fichero.Name)
	}
}
