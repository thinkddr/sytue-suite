// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import "testing"

// TestDeslizarEsLaReglaDeLaWeb repite los casos de `deslizar.test.ts` de @sytue/ui: si se
// cambia un umbral allí, este se pone en rojo hasta cambiarlo aquí. Vino de Photos con la
// regla (antes era `decidirDeslizamiento` del visor).
func TestDeslizarEsLaReglaDeLaWeb(t *testing.T) {
	casos := []struct {
		nombre    string
		dx, dy, w float32
		ms        float64
		quiero    int
	}{
		{"a la izquierda, la siguiente", -120, 4, 390, 300, 1},
		{"a la derecha, la anterior", 120, -4, 390, 300, -1},
		{"un roce lento", -40, 0, 390, 400, 0},
		{"estrecha: 55 no llegan al mínimo", -55, 0, 300, 500, 0},
		{"estrecha: 60, sí", -60, 0, 300, 500, 1},
		{"un temblor rápido", -20, 0, 390, 10, 0},
		{"en una tableta, 120 despacio no bastan", -120, 0, 820, 500, 0},
		{"en un teléfono, sí", -120, 0, 390, 500, 1},
		{"un golpe rápido pasa aunque recorra poco", -40, 0, 820, 60, 1},
		{"más vertical que horizontal", -100, 80, 390, 200, 0},
		{"horizontal por 1,4", -150, 100, 390, 200, 1},
	}
	for _, c := range casos {
		if got := DecidirDeslizamiento(c.dx, c.dy, c.w, c.ms); got != c.quiero {
			t.Errorf("%s: decidir(%v, %v, %v, %v) = %d y la web da %d", c.nombre, c.dx, c.dy, c.w, c.ms, got, c.quiero)
		}
	}
}
