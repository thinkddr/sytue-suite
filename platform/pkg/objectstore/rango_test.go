// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"errors"
	"testing"
)

func TestParsearRango(t *testing.T) {
	casos := []struct {
		rango          string
		inicio, fin    int64
		parcial, falla bool
	}{
		{"", 0, 99, false, false},
		{"bytes=0-9", 0, 9, true, false},
		{"bytes=10-", 10, 99, true, false},
		{"bytes=-10", 90, 99, true, false},
		{"bytes=-500", 0, 99, true, false},
		{"bytes=90-500", 90, 99, true, false},
		// Varios rangos u otra unidad: entero, como permite RFC 9110.
		{"bytes=0-1,5-9", 0, 99, false, false},
		{"items=0-1", 0, 99, false, false},
		{"bytes=100-", 0, 0, false, true},
		{"bytes=9-5", 0, 0, false, true},
		{"bytes=a-b", 0, 0, false, true},
		{"bytes=-0", 0, 0, false, true},
	}
	for _, c := range casos {
		inicio, fin, parcial, err := parsearRango(c.rango, 100)
		if c.falla {
			if !errors.Is(err, ErrRangoInvalido) {
				t.Errorf("%q: esperaba ErrRangoInvalido, err=%v", c.rango, err)
			}
			continue
		}
		if err != nil || inicio != c.inicio || fin != c.fin || parcial != c.parcial {
			t.Errorf("%q: %d-%d parcial=%v err=%v; esperaba %d-%d parcial=%v", c.rango, inicio, fin, parcial, err, c.inicio, c.fin, c.parcial)
		}
	}
}
