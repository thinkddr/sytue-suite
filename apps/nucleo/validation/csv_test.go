// SPDX-License-Identifier: Apache-2.0
package validation

import "testing"

// TestCSVSafe cubre el saneado de salida CSV: los arranques de fórmula se
// escapan con comilla simple y el resto del texto no se toca.
func TestCSVSafe(t *testing.T) {
	// Cada uno de los seis arranques de fórmula debe quedar escapado.
	for _, s := range []string{
		"=HYPERLINK(\"http://evil\",\"x\")",
		"+1234",
		"-1+2",
		"@SUM(A1)",
		"\tcmd",
		"\rcmd",
	} {
		got := CSVSafe(s)
		if got != "'"+s {
			t.Errorf("CSVSafe(%q) = %q; se esperaba el prefijo de comilla", s, got)
		}
	}
	// Lo que no arranca fórmula pasa intacto, incluido el vacío y un valor que
	// sólo CONTIENE (no empieza por) un carácter de fórmula.
	for _, s := range []string{"", "Ana López", "a=b", "correo@sytue.eu", "123", "'ya escapado"} {
		if got := CSVSafe(s); got != s {
			t.Errorf("CSVSafe(%q) = %q; se esperaba sin cambios", s, got)
		}
	}
}
