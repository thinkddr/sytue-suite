// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"testing"
	"time"
)

// Cada clave pasa una vez por ventana, cada una por su lado, y vuelve a pasar al cumplirse.
func TestUnaVezPorVentana(t *testing.T) {
	u := NuevaUnaVez(50 * time.Millisecond)
	if !u.Primera("a") || u.Primera("a") || !u.Primera("b") {
		t.Fatal("la primera pasa, la segunda no, y otra clave va por su lado")
	}
	time.Sleep(60 * time.Millisecond)
	if !u.Primera("a") {
		t.Error("cumplida la ventana, la clave vuelve a pasar")
	}
	var nulo *UnaVez
	for range 2 {
		if !nulo.Primera("a") {
			t.Error("un freno nil deja pasar todo")
		}
	}
}
