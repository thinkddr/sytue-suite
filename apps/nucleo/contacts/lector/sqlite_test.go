// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package lector_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// Con la libreta SQLite de Contacts (`local/libreta`), el lector lee la base: lo mismo
// que da Contacts, y lo nuevo en el acto (sin releer nada ni esperar).

// TestLaLibretaSQLiteBuscaYPaginaComoContacts: con la libreta migrada a SQLite, la de
// Mail busca, pagina y saca los cumpleaños EXACTAMENTE como la libreta de Contacts.
func TestLaLibretaSQLiteBuscaYPaginaComoContacts(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, _ := libretaVariada(t, dir) // en el almacén de registros
	quiereCumples, err := lector.Nuevo(dir, "persona").Cumpleanos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo.Close()
	x, err := libreta.Abrir(ctx, dir, "persona") // la pasa a SQLite
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	l := lector.Nuevo(dir, "persona")
	defer l.Cerrar()
	c := l.Libreta()
	for _, q := range []string{"luis", "LUIS", "pérez", "ñu", "pepe", "ACME ibérica", "@ejemplo", "600", "avila", "a", "  marta  ", "luisa", "antiguo", "zzz", "", "   "} {
		for _, limite := range []int{8, 1, 0, -3, 501} {
			quiere, errQuiere := x.Search(ctx, q, limite)
			tiene, errTiene := c.Search(ctx, q, limite)
			if (errQuiere != nil) != (errTiene != nil) || !slices.Equal(idsDe(tiene), idsDe(quiere)) {
				t.Errorf("buscar %q (%d): %v %v ≠ %v %v", q, limite, idsDe(tiene), errTiene, idsDe(quiere), errQuiere)
			}
		}
	}
	for _, p := range [][2]int{{500, 0}, {2, 0}, {2, 2}, {2, 4}, {0, 0}, {3, 1000}, {3, -2}} {
		quiere, sigQuiere, _ := x.All(ctx, p[0], p[1])
		tiene, sigTiene, err := c.All(ctx, p[0], p[1])
		if err != nil || !slices.Equal(idsDe(tiene), idsDe(quiere)) || sigTiene != sigQuiere {
			t.Errorf("página %v: %v %d %v ≠ %v %d", p, idsDe(tiene), sigTiene, err, idsDe(quiere), sigQuiere)
		}
	}
	if ds, err := c.Buscar(ctx, "luis", 5); err != nil || len(ds) != 2 || ds[0].Correo != "luis@ejemplo.eu" {
		t.Fatalf("direcciones: %+v %v", ds, err)
	}
	if cs, err := l.Cumpleanos(ctx); err != nil || !slices.Equal(cs, quiereCumples) {
		t.Fatalf("cumpleaños: %+v ≠ %+v (%v)", cs, quiereCumples, err)
	}
}

// TestLaLibretaSQLiteVeLoNuevoEnElActo: lo que Contacts escribe en la base lo ve la libreta
// de Mail en la siguiente letra, sin relectura detrás y sin almacén de registros (una
// instalación nueva no lo tiene).
func TestLaLibretaSQLiteVeLoNuevoEnElActo(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	x, err := libreta.Abrir(ctx, dir, "persona")
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	lb, err := x.CreateAddressBook(ctx, "Contactos")
	if err != nil {
		t.Fatal(err)
	}
	l := lector.Nuevo(dir, "persona")
	defer l.Cerrar()
	c := l.Libreta()
	c.Precargar()
	if cs, err := c.Search(ctx, "marta", 8); err != nil || cs != nil {
		t.Fatalf("libreta vacía: %v %v", cs, err)
	}
	if cs, n, err := c.All(ctx, 10, 0); err != nil || cs != nil || n != 0 {
		t.Fatalf("libreta vacía, la página: %v %d %v", cs, n, err)
	}
	if _, err := x.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Marta Gil", Emails: []api.ContactEmail{{Email: "marta@ejemplo.eu"}}}); err != nil {
		t.Fatal(err)
	}
	inicio := time.Now()
	ds, err := c.Buscar(ctx, "mar", 5)
	if err != nil || len(ds) != 1 || ds[0].Correo != "marta@ejemplo.eu" {
		t.Fatalf("lo nuevo no aparece en el acto: %+v %v", ds, err)
	}
	if d := time.Since(inicio); d > 2*time.Second {
		t.Fatalf("autocompletar tardó %v", d)
	}
	if _, err := x.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Marta Segunda"}); err != nil {
		t.Fatal(err)
	}
	if cs, _ := c.Search(ctx, "marta", 8); len(cs) != 2 {
		t.Fatalf("lo segundo no aparece: %+v", cs)
	}
	if _, err := os.Stat(filepath.Join(dir, "registros.jsonl")); !os.IsNotExist(err) {
		t.Fatal("leer creó el almacén de registros")
	}
}

// TestElLectorPasaALaBaseCuandoContactsMigra: antes de que Contacts se abra con la base,
// el lector lee el almacén de registros; en cuanto la pasa a SQLite, lee la base, y lo
// que Contacts escriba ya va a ella (el almacén viejo no cambia).
func TestElLectorPasaALaBaseCuandoContactsMigra(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, _ := libretaVariada(t, dir)
	l := lector.Nuevo(dir, "persona")
	defer l.Cerrar()
	c := l.Libreta()
	if cs, err := c.Search(ctx, "luis", 8); err != nil || len(cs) != 1 {
		t.Fatalf("en el almacén de antes: %+v %v", cs, err)
	}
	repo.Close()
	diario, _ := os.ReadFile(filepath.Join(dir, "registros.jsonl"))
	x, err := libreta.Abrir(ctx, dir, "persona")
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	ls, err := x.AddressBooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.Create(ctx, ls[0].ID, api.ContactInput{FormattedName: "Luis Nuevo"}); err != nil {
		t.Fatal(err)
	}
	if cs, err := c.Search(ctx, "luis", 8); err != nil || len(cs) != 2 {
		t.Fatalf("tras migrar, la base: %+v %v", cs, err)
	}
	if despues, _ := os.ReadFile(filepath.Join(dir, "registros.jsonl")); !bytes.Equal(despues, diario) {
		t.Fatal("escribir tras migrar tocó el almacén de antes")
	}
}
