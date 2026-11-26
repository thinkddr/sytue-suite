// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package lector_test

import (
	"context"
	"fmt"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// El lector trae todos los contactos, sin repetir. Cuando los pedía por páginas de
// `All`, se quedaba con la primera, porque tomaba el desplazamiento de la siguiente por el
// total (visto en rojo con la página de dos: traía dos de siete); ahora los toma de una
// vez de la instantánea (`Snapshot.ContactosVisibles`).
func TestElLectorTraeMasDeUnaPagina(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	rec, err := almacen.Abrir(context.Background(), dir, "contacts", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := contacts.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := almacen.Preparar(ctx, repo); err != nil {
		t.Fatal(err)
	}
	lb, err := repo.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	const cuantos = 7
	for i := range cuantos {
		if _, err := repo.Create(ctx, lb.ID, api.ContactInput{FormattedName: fmt.Sprintf("Persona %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	repo.Close()
	rec.Cerrar()

	todos, err := lector.Nuevo(dir, "instalacion-1").Contactos(ctx)
	if err != nil || len(todos) != cuantos {
		t.Fatalf("el lector trae %d de %d: %v", len(todos), cuantos, err)
	}
	vistos := map[string]bool{}
	for _, c := range todos {
		vistos[c.ID] = true
	}
	if len(vistos) != cuantos {
		t.Fatalf("repetidos: %d distintos", len(vistos))
	}
}
