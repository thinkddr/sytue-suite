// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import "testing"

// TestLosNumerosVanALaDerecha: sin alineación propia, la celda se pinta con la de
// «General» de cualquier hoja de cálculo —los números a la derecha y lo demás a la
// izquierda— y la propia manda. Iban todos a la izquierda (auditoría del 2026-11-24).
// Gemelo de `alineacionPintada` en la web (packages/office/test/sheets-format.test.ts).
func TestLosNumerosVanALaDerecha(t *testing.T) {
	lz := libroDePrueba(t, `{"active":0,"sheets":[{"name":"Datos","cells":{`+
		`"A1":"Alquiler","B1":"1200","C1":"=B1*2","D1":"<div style=\"text-align: center;\">12</div>",`+
		`"E1":"<b>Total</b>","F1":"=A1"}}]}`)
	l := lz.Libro
	for _, c := range []struct {
		col    int
		quiero string
	}{
		{0, ""},       // texto
		{1, "right"},  // número escrito
		{2, "right"},  // fórmula que da un número
		{3, "center"}, // alineación propia
		{4, ""},       // texto en negrita
		{5, ""},       // fórmula que da texto
	} {
		if got := l.AlinearPintado(c.col, 0); got != c.quiero {
			t.Errorf("la celda %d,0 (%q) se alinea %q; quiero %q", c.col, l.Crudo(c.col, 0), got, c.quiero)
		}
	}
}
