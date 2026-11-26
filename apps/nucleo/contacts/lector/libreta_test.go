// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package lector_test

import (
	"context"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// La libreta en memoria da las direcciones que casan con lo escrito (la principal de
// cada contacto primero, sin repetir y hasta el límite), sin almacén no da nada ni
// falla, y lo que Contacts escribe después se ve.
func TestLibretaBuscaDirecciones(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	lib := lector.Nuevo(dir, "instalacion-1").Libreta()
	if ds, err := lib.Buscar(ctx, "lu", 5); err != nil || len(ds) != 0 {
		t.Fatal("sin almacén", ds, err)
	}
	rec, err := almacen.Abrir(context.Background(), dir, "contacts", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Cerrar()
	repo, err := contacts.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := almacen.Preparar(ctx, repo); err != nil {
		t.Fatal(err)
	}
	lb, err := repo.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []api.ContactInput{
		{FormattedName: "Luis Pérez", Emails: []api.ContactEmail{{Email: "luis@casa.example"}, {Email: "luis@trabajo.example", IsPrimary: true}}},
		{FormattedName: "Lucía Gil", Emails: []api.ContactEmail{{Email: "LUIS@casa.example"}, {Email: "lucia@example.org"}}},
		{FormattedName: "Marta Ruiz", Emails: []api.ContactEmail{{Email: "marta@example.org"}}},
	} {
		if _, err := repo.Create(ctx, lb.ID, c); err != nil {
			t.Fatal(err)
		}
	}
	lib = lector.Nuevo(dir, "instalacion-1").Libreta()
	ds, err := lib.Buscar(ctx, "lu", 5)
	correos := make([]string, 0, len(ds))
	for _, d := range ds {
		correos = append(correos, d.Correo)
	}
	// La misma dirección en dos contactos sale una vez; de Luis, su principal.
	if err != nil || len(ds) != 3 || ds[0].Nombre == "" || !slices.Contains(correos, "lucia@example.org") || !slices.Contains(correos, "luis@trabajo.example") {
		t.Fatalf("direcciones: %+v %v", ds, err)
	}
	if ds, _ := lib.Buscar(ctx, "lu", 1); len(ds) != 1 {
		t.Fatalf("límite: %+v", ds)
	}
	if todos, err := lib.Contactos(ctx); err != nil || len(todos) != 3 {
		t.Fatal(todos, err)
	}
}
