// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "testing"

// TestDestinoHyperlink: el valor de la celda es el nombre visible, pero el editor necesita
// el destino para abrirlo; literal o calculado con las referencias de la celda.
func TestDestinoHyperlink(t *testing.T) {
	e := New(Options{Sheets: []string{"H"}})
	e.SetCellAt("H", 0, 0, "https://ejemplo.org")
	e.SetCellAt("H", 1, 0, `=HYPERLINK("https://ejemplo.org/precios";"Precios")`)
	e.SetCellAt("H", 1, 1, `=hyperlink(A1&"/ficha?id="&2)`)
	e.SetCellAt("H", 1, 2, `=UPPER(HYPERLINK("https://ejemplo.org"))`)
	e.SetCellAt("H", 1, 3, `=HYPERLINK(42)`)
	e.SetCellAt("H", 1, 4, `=A1`)

	if v := e.ValueAt("H", 1, 0); v != "Precios" {
		t.Fatalf("el valor sigue siendo el nombre: %v", v)
	}
	casos := []struct {
		fila    int
		destino string
		ok      bool
	}{
		{0, "https://ejemplo.org/precios", true},
		{1, "https://ejemplo.org/ficha?id=2", true},
		{2, "", false}, // HYPERLINK dentro de otra función: no es un enlace
		{3, "", false}, // el destino no es texto
		{4, "", false}, // no hay HYPERLINK
		{9, "", false}, // celda vacía
	}
	for _, c := range casos {
		d, ok := e.DestinoHyperlink("H", 1, c.fila)
		if d != c.destino || ok != c.ok {
			t.Errorf("fila %d: (%q, %v), quería (%q, %v)", c.fila, d, ok, c.destino, c.ok)
		}
	}
	// Un literal no es una fórmula: su URL la reconoce el editor, no el motor.
	if _, ok := e.DestinoHyperlink("H", 0, 0); ok {
		t.Error("un literal no tiene destino de HYPERLINK")
	}
}
