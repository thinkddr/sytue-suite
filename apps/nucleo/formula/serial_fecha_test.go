// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "testing"

// TestSerialDeFechaEsElDeLaWeb: los mismos números que `serialDeHoy` de la web
// (`sheets/atajosHoja.test.tsx`), día fantasma incluido.
func TestSerialDeFechaEsElDeLaWeb(t *testing.T) {
	for _, c := range []struct{ a, m, d, quiero int }{
		{2026, 11, 13, 46339},
		{1900, 3, 1, 61},
		{1900, 1, 1, 1},
	} {
		if got := SerialDeFecha(c.a, c.m, c.d); got != c.quiero {
			t.Errorf("SerialDeFecha(%d, %d, %d) = %d, quiero %d", c.a, c.m, c.d, got, c.quiero)
		}
	}
}
