// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// «Eliminar para siempre» un contacto por la fachada de la pantalla lo quita del disco
// (la base de la libreta y su diario) sin cerrar la app, y lo demás se sigue leyendo.
func TestBorrarUnContactoDelTodoSeVaDelDisco(t *testing.T) {
	antes := libreta.EsperaVaciar
	libreta.EsperaVaciar = 20 * time.Millisecond
	t.Cleanup(func() { libreta.EsperaVaciar = antes })

	dir := t.TempDir()
	ctx := t.Context()
	repo, err := libreta.Abrir(ctx, dir, "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	svc, err := api.NewContacts("", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	libro, err := svc.CreateAddressBook(ctx, "Contactos")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, libro.ID, api.ContactInput{FormattedName: "Ana Ruiz"}); err != nil {
		t.Fatal(err)
	}
	borrado, err := svc.Create(ctx, libro.ID, api.ContactInput{FormattedName: "Ex Pareja Secreta"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, borrado.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Purge(ctx, borrado.ID); err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(10 * time.Second)
	for {
		sigue := false
		for _, f := range []string{libreta.Fichero, libreta.Fichero + "-wal"} {
			raw, _ := os.ReadFile(filepath.Join(dir, f))
			sigue = sigue || bytes.Contains(raw, []byte("Pareja Secreta")) || bytes.Contains(raw, []byte("pareja secreta"))
		}
		if !sigue {
			break
		}
		if time.Now().After(limite) {
			t.Fatal("lo borrado del todo sigue en el disco con la app abierta")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cs, _, err := svc.All(ctx, 50, 0)
	if err != nil || len(cs) != 1 || cs[0].FormattedName != "Ana Ruiz" {
		t.Fatal("tras borrar del todo", cs, err)
	}
}
