// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"fmt"
	"image/color"
)

// RellenoDeRuta es un glifo de SOLO relleno: `<svg fill="currentColor"><path d/></svg>`
// sin trazo, como el play y la pausa del pase de Sytue Photos.
//
// No es TrazoDeRutaRelleno: ese traza además el contorno con el grosor de `Glifo`, que
// engorda la forma —un triángulo de play que en la web mide 12×14 salía más grande—.
func RellenoDeRuta(d string, lado float32, c color.NRGBA) *Dibujo {
	if d == "" {
		panic("iconos: RellenoDeRuta sin ruta")
	}
	return nuevoDibujo(lado, c, func(p pintura) []byte {
		c := p.tinta
		return fmt.Appendf(nil,
			`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`+
				`<path d="%s" fill="#%02x%02x%02x" fill-opacity="%.4f"/></svg>`,
			rejilla, rejilla, rejilla, rejilla, d, c.R, c.G, c.B, float32(c.A)/255)
	})
}
