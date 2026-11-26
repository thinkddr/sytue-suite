// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// abrirLocal abre el repositorio de Contacts, sin tocarlo, sobre el puerto en claro de
// Sytue Office (apps/nucleo/local/almacen), como hace `cmd-local/contacts`.
func abrirLocal(t *testing.T, dir string) (*contacts.Repository, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "contacts", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := contacts.New(rec.Config)
	if err != nil {
		rec.Cerrar()
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal("preparar:", err)
	}
	return repo, func() { repo.Close(); rec.Cerrar() }
}

const tarjetaDePrueba = "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\nEMAIL:ada@ejemplo.org\r\nEND:VCARD\r\n"

func TestContactsLocalSobreviveAReabrir(t *testing.T) {
	dir := t.TempDir()
	repo, cerrar := abrirLocal(t, dir)
	ctx := t.Context()
	libros, err := repo.AddressBooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var libreta string
	if len(libros) > 0 {
		libreta = libros[0].ID
	} else {
		lb, err := repo.CreateAddressBook(ctx, "Personal")
		if err != nil {
			t.Fatal(err)
		}
		libreta = lb.ID
	}
	if _, err := repo.Create(ctx, libreta, api.ContactInput{FormattedName: "Grace Hopper", GivenName: "Grace", FamilyName: "Hopper"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ImportCard(ctx, libreta, tarjetaDePrueba); err != nil {
		t.Fatal("importar vCard", err)
	}
	if n, _, err := repo.ImportCSV(ctx, libreta, []byte("Name,E-mail 1 - Value\r\nAlan Turing,alan@ejemplo.org\r\n")); err != nil || n != 1 {
		t.Fatal("importar CSV", n, err)
	}
	cerrar()

	repo2, cerrar2 := abrirLocal(t, dir)
	defer cerrar2()
	todos, _, err := repo2.All(ctx, 100, 0)
	if err != nil || len(todos) != 3 {
		t.Fatal("contactos tras reabrir", todos, err)
	}
	hits, err := repo2.Search(ctx, "ada", 10)
	if err != nil || len(hits) != 1 || hits[0].FormattedName != "Ada Lovelace" {
		t.Fatal("buscar tras reabrir", hits, err)
	}
}
