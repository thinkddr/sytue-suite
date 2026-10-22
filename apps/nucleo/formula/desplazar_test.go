// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "testing"

// TestShiftRefsEsElDeLaWeb son, caso a caso, los de `shiftRefs` en
// `packages/office/test/sheets-format.test.ts` y `sheets-structured.test.ts`: la web y el
// escritorio tienen que desplazar igual la misma fórmula, o un libro pegado en uno
// calcula otra cosa abierto en el otro.
func TestShiftRefsEsElDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		formula string
		dc, dr  int
		quiero  string
	}{
		{"=A1+B$2*$C3", 1, 2, "=B3+C$2*$C5"},
		{"=$A$1", 5, 5, "=$A$1"},
		{`=SUMA(A1:B2;"A1:B2")`, 0, 1, `=SUMA(A2:B3;"A1:B2")`},
		{"='Hoja 2'!A1+Hoja3!B2", 1, 0, "='Hoja 2'!B1+Hoja3!C2"},
		{"=(A1+B1)*C1", 0, 1, "=(A2+B2)*C2"},
		{"=-A1%", 0, 1, "=-A2%"},
		{`=SI(A1>2;"sí";B1&"x")`, 2, 0, `=SI(C1>2;"sí";D1&"x")`},
		{"=A1", 0, -1, "#REF!"},
		{"=FOO(", 1, 1, "=FOO("},
		{"=SUM(A:A)", 2, 0, "=SUM(C:C)"},
		{"=SUM(A:A)", 0, 5, "=SUM(A:A)"},
		{"=SUM(A:C)", 1, 0, "=SUM(B:D)"},
		{"=SUM($A:$A)", 2, 0, "=SUM($A:$A)"},
		{"=SUM(1:1)", 0, 2, "=SUM(3:3)"},
		{"=SUM(3:5)", 0, -1, "=SUM(2:4)"},
		{"=SUM(Ventas[Ventas])", 3, 5, "=SUM(Ventas[Ventas])"},
		{"=[@Coste]", 1, 1, "=[@Coste]"},
		{"=Ventas[#Totales]", 4, 4, "=Ventas[#Totales]"},
		{"=A1+Ventas[[#Cabeceras],[Coste]]", 0, 1, "=A2+Ventas[[#Cabeceras],[Coste]]"},
		// Lo que el serializador tiene que conservar para volver al MISMO árbol.
		{"=2^3^2", 0, 0, "=2^3^2"},
		{"=(2^3)^2", 0, 0, "=(2^3)^2"},
		{"=A1-(B1-C1)", 0, 0, "=A1-(B1-C1)"},
		{"=0.1+1E+21", 0, 0, "=0.1+1e+21"},
	} {
		if got := ShiftRefs(c.formula, c.dc, c.dr); got != c.quiero {
			t.Errorf("ShiftRefs(%q, %d, %d) = %q y la web da %q", c.formula, c.dc, c.dr, got, c.quiero)
		}
	}
}
