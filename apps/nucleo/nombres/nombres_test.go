// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nombres

import "testing"

func TestLimpio(t *testing.T) {
	for entrada, quiero := range map[string]string{
		"factura\u202Efdp.exe": "facturafdp.exe",
		"a\x00b\nc.txt":        "abc.txt",
		"\u2066x\u2069.pdf":    "x.pdf",
		"Informe año 2026.pdf": "Informe año 2026.pdf",
		"👍🏽.png":               "👍🏽.png",
	} {
		if got := Limpio(entrada); got != quiero {
			t.Errorf("Limpio(%q) = %q, quiero %q", entrada, got, quiero)
		}
	}
}
