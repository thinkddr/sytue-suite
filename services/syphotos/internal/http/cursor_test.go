// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// EL CURSOR TIENE QUE VOLVER EXACTO. Si pierde precisión, el desempate por id no
// basta —una ráfaga comparte segundo— y la página siguiente repite o se salta
// fotos. Sin error: solo mal.
func TestCursor_idaYVuelta(t *testing.T) {
	// Con nanosegundos: es la precisión que distingue dos fotos de una ráfaga.
	c := store.Cursor{
		Fecha: time.Date(2026, 7, 1, 12, 34, 56, 123456789, time.UTC),
		ID:    uuid.MustParse("6f1c8f2e-3a4b-4c5d-8e9f-0a1b2c3d4e5f"),
	}
	vuelta, err := descifrarCursor(cifrarCursor(c))
	if err != nil {
		t.Fatal(err)
	}
	if !vuelta.Fecha.Equal(c.Fecha) {
		t.Errorf("la fecha volvió %v, se mandó %v", vuelta.Fecha, c.Fecha)
	}
	if vuelta.ID != c.ID {
		t.Errorf("el id volvió %v", vuelta.ID)
	}
}

// La zona no puede cambiar la posición: el mismo instante en dos zonas es el
// mismo sitio de la línea de tiempo.
func TestCursor_laZonaNoMueveLaPosicion(t *testing.T) {
	madrid := time.FixedZone("CEST", 2*3600)
	id := uuid.New()
	enUTC := store.Cursor{Fecha: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC), ID: id}
	enMadrid := store.Cursor{Fecha: time.Date(2026, 7, 1, 12, 0, 0, 0, madrid), ID: id}
	if cifrarCursor(enUTC) != cifrarCursor(enMadrid) {
		t.Error("el mismo instante en dos zonas dio dos cursores distintos")
	}
}

// UN CURSOR MANIPULADO NO PUEDE TUMBAR NADA: lo escribe quien quiera en la URL.
func TestCursor_rechazaLoQueNoEs(t *testing.T) {
	casos := []string{
		"",
		"no-es-base64!!",
		"YWJj",                       // base64 válido, contenido sin «|»
		"MjAyNi0wNy0wMXxub28",        // fecha bien, id ilegible
		"bm8tZXMtZmVjaGF8Ni1uby1lcw", // ninguna de las dos
	}
	for _, c := range casos {
		if _, err := descifrarCursor(c); err == nil {
			t.Errorf("se aceptó el cursor %q", c)
		}
	}
}
