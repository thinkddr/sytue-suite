// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package sesion

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/notas"
)

func TestSubirCanceladoNoOcultaLosBorradoresQueSiguenPendientes(t *testing.T) {
	aparte(t)
	store, err := Almacen()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write("alice/org", notas.EspacioBorradores, "one", "unsent private draft"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if n := new(Suite).SubirNotepads(ctx); n != 1 {
		t.Fatal("failed unlock/upload reported unsent data as empty", n)
	}
}
