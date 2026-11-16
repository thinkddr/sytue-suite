// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"testing"
)

func TestReadingPrefsPorDefectoSonValidas(t *testing.T) {
	if err := ReadingPrefsPorDefecto.Validar(); err != nil {
		t.Fatalf("las de por defecto no validan: %v", err)
	}
	// El plan: tras archivar, la más antigua; derecha archiva, izquierda a la papelera.
	if ReadingPrefsPorDefecto != (ReadingPrefs{AutoAdvance: "older", SwipeRight: "archive", SwipeLeft: "trash"}) {
		t.Fatalf("por defecto = %+v", ReadingPrefsPorDefecto)
	}
}

func TestNormalizarRemitente(t *testing.T) {
	casos := map[string]string{
		"ana@x.eu":            "ana@x.eu",
		"  Ana@Boletin.EU \n": "ana@boletin.eu",
		"ana+news@x.eu":       "ana+news@x.eu",
	}
	for entrada, want := range casos {
		got, err := NormalizarRemitente(entrada)
		if err != nil || got != want {
			t.Fatalf("%q ⇒ %q (%v), quiero %q", entrada, got, err, want)
		}
	}
	for _, mala := range []string{"", "ana", "@x.eu", "ana@", "a@b@c", "ana @x.eu", "<ana@x.eu>", "a,b@x.eu"} {
		if _, err := NormalizarRemitente(mala); !errors.Is(err, ErrInvalidAddress) {
			t.Fatalf("%q: err = %v, quiero ErrInvalidAddress", mala, err)
		}
	}
}
