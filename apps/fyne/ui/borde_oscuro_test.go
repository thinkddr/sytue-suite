// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// TestElBordeSeComponeSobreSuFondo: un borde de la hoja es `rgba(tinta, α)` y la web lo
// ve sobre lo que tenga detrás. El número está escrito a mano con la cuenta del navegador
// —rgba(231, 239, 239, .11) sobre #0a1011— y no sale de `SobreSRGB`: si saliera, el test
// se comprobaría a sí mismo. VISTO EN ROJO antes del arreglo (2026-09-27, con la paleta
// de entonces): la caja pintaba el borde aplanado sobre `surface` y el filete sobre `bg`
// también.
func TestElBordeSeComponeSobreSuFondo(t *testing.T) {
	tema.Elegir("oscuro")
	t.Cleanup(func() { tema.Elegir("") })
	v := tema.Oscuro
	hex := func(n uint32) color.NRGBA {
		return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 0xff}
	}
	// El filete de la cabecera del ERP, `border-b border-border` sobre `bg`: #222829. A ±1
	// por canal: la hoja dice α 0,11 y un byte de alfa solo llega a 28/255 = 0,1098.
	if got, web := BordeSobre("border", "bg", v), hex(0x222829); !aUnNivel(got, web) {
		t.Errorf("border sobre bg = %v, la web pinta #222829", got)
	}
	// El borde de una caja con relleno opaco va sobre ese relleno, no sobre `surface`.
	c := NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioLG)
	quiero := tokens.SobreSRGB(tokens.Crudo("border", v), tema.Color("surface-raised", v))
	if got := c.fuera.FillColor; got != color.Color(quiero) {
		t.Errorf("borde de la caja sobre surface-raised = %v, quiero %v", got, quiero)
	}
	c.Pintar(tema.Color("surface-raised", v), tema.Color("border", v))
	if got := c.fuera.FillColor; got != color.Color(quiero) {
		t.Errorf("tras Pintar, el borde = %v, quiero %v", got, quiero)
	}
	// Sobre `surface` no cambia nada: es donde ya estaba aplanado.
	c = NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG)
	if got := c.fuera.FillColor; got != color.Color(tema.Color("border", v)) {
		t.Errorf("sobre surface el borde ha cambiado: %v", got)
	}
}

func aUnNivel(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return x-y <= 1 || y-x <= 1 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && a.A == b.A
}
