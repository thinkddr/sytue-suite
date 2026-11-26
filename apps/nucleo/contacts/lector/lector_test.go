// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package lector_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// Otra app lee los contactos (y sus cumpleaños) mientras Contacts está abierto, ve lo
// que Contacts escribe después y no escribe nada en su almacén.
func TestElLectorEntreAppsEsDeSoloLectura(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	if l, err := lector.Nuevo(dir, "instalacion-1").Contactos(ctx); err != nil || len(l) != 0 {
		t.Fatal("sin almacén no hay contactos ni error", l, err)
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
	if _, err := repo.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Ada Lovelace", Birthday: "1815-12-10"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Sin año", BirthdayNoYear: "--02-29"}); err != nil {
		t.Fatal(err)
	}
	antes, err := os.ReadFile(filepath.Join(dir, "registros.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	l := lector.Nuevo(dir, "instalacion-1")
	cumples, err := l.Cumpleanos(ctx)
	if err != nil || len(cumples) != 2 {
		t.Fatal("cumpleaños", cumples, err)
	}
	if cumples[0].Nombre != "Sin año" || cumples[0].Mes != time.February || cumples[0].Dia != 29 || cumples[0].Ano != 0 {
		t.Fatal("el de febrero, sin año, primero", cumples[0])
	}
	if cumples[1].Ano != 1815 || cumples[1].Mes != time.December || cumples[1].Dia != 10 {
		t.Fatal("el de Ada", cumples[1])
	}
	if hits, err := l.Buscar(ctx, "ada", 10); err != nil || len(hits) != 1 {
		t.Fatal("buscar", hits, err)
	}
	despues, _ := os.ReadFile(filepath.Join(dir, "registros.jsonl"))
	if !bytes.Equal(antes, despues) {
		t.Fatal("el lector ha escrito en el almacén de Contacts")
	}

	// Lo que Contacts escribe después, el lector lo ve en la lectura siguiente.
	if _, err := repo.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Grace Hopper", Birthday: "1906-12-09"}); err != nil {
		t.Fatal(err)
	}
	if todos, err := l.Contactos(ctx); err != nil || len(todos) != 3 {
		t.Fatal("tras escribir Contacts", todos, err)
	}
	// Y Contacts sigue escribiendo con normalidad.
	if _, err := repo.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Alan Turing"}); err != nil {
		t.Fatal("Contacts sigue escribiendo", err)
	}
}
