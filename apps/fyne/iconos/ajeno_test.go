// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"image"
	"testing"
)

// TestUnSVGAjenoNoTumbaNadaYNoSeDeforma: el logotipo de una aplicación conectada pasa
// por las correcciones de las marcas —un `rect` con solo `rx` sale redondeado por los
// cuatro lados—, encaja sin deformarse en su cuadro y, si no se deja leer, se descarta
// sin pánico.
//
// Visto en ROJO a mano (en el frontal): con `normaliza` devolviendo el SVG sin tocar,
// la esquina de abajo del `rect` con `rx` sale en ángulo recto, que es el fallo de
// `oksvg` que ya tenían las marcas.
func TestUnSVGAjenoNoTumbaNadaYNoSeDeforma(t *testing.T) {
	ancho := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 20">` +
		`<rect x="0" y="0" width="40" height="20" rx="8" fill="#123456"/></svg>`)
	d, ok := DeSVGAjeno(ancho, 28)
	if !ok {
		t.Fatal("un SVG válido se ha descartado")
	}
	if m := d.Medida(); m.Width != 28 || m.Height != 14 {
		t.Errorf("mide %v; con `meet` un 2:1 en 28×28 son 28×14", m)
	}
	img, _ := d.generar(40, 20).(*image.NRGBA)
	// La esquina de abajo a la izquierda: con `ry` = `rx` el píxel del borde es
	// transparente; sin la corrección, `oksvg` pinta la esquina en ángulo recto.
	if a := img.NRGBAAt(0, 19).A; a != 0 {
		t.Errorf("la esquina de abajo sale pintada (alfa %d): el `rect` no lleva el `ry` que falta", a)
	}
	for _, malo := range []string{
		`no es un svg`,
		`<svg xmlns="http://www.w3.org/2000/svg"><rect width="4" height="4"/></svg>`, // sin tamaño
	} {
		if _, ok := DeSVGAjeno([]byte(malo), 28); ok {
			t.Errorf("%q se ha dado por bueno", malo)
		}
	}
}
