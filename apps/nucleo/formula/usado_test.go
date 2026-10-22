// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "testing"

// TestUsedRange: la última columna y fila usadas cuentan las celdas con contenido y los
// derrames, y una hoja vacía da (-1, -1).
func TestUsedRange(t *testing.T) {
	e := New(Options{Sheets: []string{"A", "B"}})
	if c, f := e.UsedRange("A"); c != -1 || f != -1 {
		t.Errorf("hoja vacía: (%d, %d)", c, f)
	}
	e.SetCellAt("A", 2, 4, "x")
	if c, f := e.UsedRange("A"); c != 2 || f != 4 {
		t.Errorf("con una celda en C5: (%d, %d)", c, f)
	}
	e.SetCellAt("A", 0, 0, "=SEQUENCE(8)")
	e.ValueAt("A", 0, 0) // el derrame se calcula al leer
	if c, f := e.UsedRange("A"); c != 2 || f != 7 {
		t.Errorf("con un derrame de A1 a A8: (%d, %d)", c, f)
	}
	if c, f := e.UsedRange("B"); c != -1 || f != -1 {
		t.Errorf("la otra hoja sigue vacía: (%d, %d)", c, f)
	}
}
