// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package saneado

import "testing"

// TestLaReglaDelDocumento: lo legítimo vuelve SIN TOCAR (la misma cadena) y lo peligroso
// se va: los manejadores, los esquemas de URL que no son de la lista y lo que se quita con
// su contenido. Las pruebas de contrato con el editor y la importación siguen en
// services/docs/internal/domain, que la aplica al guardar.
func TestLaReglaDelDocumento(t *testing.T) {
	legitimo := `<!--sytue-track--><p style="color: red">a <b>b</b> <img src="data:image/png;base64,AA"></p>`
	if got := HTML(legitimo); got != legitimo {
		t.Errorf("lo legítimo cambia: %q", got)
	}
	for entrada, quiero := range map[string]string{
		`<p onclick="x()">a</p>`:                        `<p>a</p>`,
		`<a href="javascript:alert(1)">a</a>`:           `<a>a</a>`,
		`<script>robar()</script><p>a</p><!-- nota -->`: `<p>a</p>`,
		`<img src="vbscript:x" alt="y">`:                `<img alt="y">`,
		`<o:p>texto</o:p>`:                              `texto`,
	} {
		if got := HTML(entrada); got != quiero {
			t.Errorf("HTML(%q) = %q, y quiero %q", entrada, got, quiero)
		}
	}
	if URLAdmitida(" JavaScript:alert(1)", false) || !URLAdmitida("data:image/png;base64,AA", true) || URLAdmitida("data:image/png;base64,AA", false) {
		t.Error("URLAdmitida no aplica la regla 4")
	}
}
