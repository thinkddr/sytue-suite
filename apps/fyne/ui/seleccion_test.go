// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"slices"
	"testing"
)

func marcadas(sel map[string]bool, ids []string) []string {
	var out []string
	for _, id := range ids {
		if sel[id] {
			out = append(out, id)
		}
	}
	return out
}

func mismo(s string) string { return s }

// TestMarcarRangoEnCualquierOrden: del ancla al pulsado, en cualquier orden, sin
// desmarcar lo que ya estaba y saltándose lo que no tiene id.
func TestMarcarRangoEnCualquierOrden(t *testing.T) {
	ids := []string{"a", "b", "", "d", "e"}
	sel := map[string]bool{"e": true}
	MarcarRango(sel, ids, mismo, 3, 1)
	if got := marcadas(sel, ids); !slices.Equal(got, []string{"b", "d", "e"}) {
		t.Errorf("el rango marca %q", got)
	}
	// Un ancla que se quedó fuera de la lista no inventa posiciones.
	MarcarRango(sel, ids, mismo, 0, 40)
	if len(sel) != 4 || sel[""] {
		t.Errorf("con el índice fuera de la lista marca %v", sel)
	}
}

// TestMarcarConTramoDeLaTercerAlaDecima es el criterio de la pieza: marcar la 3 y Mayús
// + clic en la 10 deja marcadas de la 3 a la 10, y el ancla sigue en la 3.
func TestMarcarConTramoDeLaTercerAlaDecima(t *testing.T) {
	ids := make([]string, 12)
	for i := range ids {
		ids[i] = fmt.Sprintf("f%d", i+1)
	}
	sel := map[string]bool{}
	ancla := MarcarConTramo(sel, ids, mismo, 2, -1, false)
	if ancla != 2 || !slices.Equal(marcadas(sel, ids), []string{"f3"}) {
		t.Fatalf("el primer toque deja %v con el ancla en %d", sel, ancla)
	}
	ancla = MarcarConTramo(sel, ids, mismo, 9, ancla, true)
	want := []string{"f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10"}
	if ancla != 2 || !slices.Equal(marcadas(sel, ids), want) {
		t.Errorf("Mayús + clic en la 10 deja %v con el ancla en %d", marcadas(sel, ids), ancla)
	}
	// Sin Mayús, un toque desmarca y mueve el ancla.
	if ancla = MarcarConTramo(sel, ids, mismo, 4, ancla, false); ancla != 4 || sel["f5"] {
		t.Errorf("sin Mayús no desmarca la 5 (ancla %d)", ancla)
	}
	// Con Mayús pero sin ancla, es un toque normal.
	sel = map[string]bool{}
	if ancla = MarcarConTramo(sel, ids, mismo, 6, -1, true); ancla != 6 || !sel["f7"] || len(sel) != 1 {
		t.Errorf("Mayús sin ancla deja %v con el ancla en %d", sel, ancla)
	}
}
