// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import "testing"

// TestElGrosorEncogeConElGrupo: una raya de `stroke-width="4"` dentro de un
// `<g transform="scale(0.5)">` mide 2 en el navegador, porque el grosor se pinta en el
// sistema del grupo. `oksvg` escala los puntos y no el grosor: sin la corrección salían
// 4 filas, y el estampador del glifo de PDF era una mancha.
func TestElGrosorEncogeConElGrupo(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">` +
		`<g transform="scale(0.5)"><path d="M0 24H48" fill="none" stroke="#000" stroke-width="4"/></g></svg>`)
	img := rasteriza(svg, 24, 24)
	filas := 0
	for y := range 24 {
		if img.NRGBAAt(12, y).A > 127 {
			filas++
		}
	}
	if filas != 2 {
		t.Errorf("la raya ocupa %d filas; en el navegador, 2", filas)
	}
}
