// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tema

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	temafyne "fyne.io/fyne/v2/theme"
)

// TestSinBarrasEscondeLaBarraYLaSombra: el `[scrollbar-width:none]` de una tira que se
// desliza de lado. VISTO EN ROJO sin el caso de la barra: se pintaba la del tema.
func TestSinBarrasEscondeLaBarraYLaSombra(t *testing.T) {
	for _, n := range []struct {
		nombre string
		c      color.Color
	}{
		{"barra", SinBarras{}.Color(temafyne.ColorNameScrollBar, temafyne.VariantLight)},
		{"carril", SinBarras{}.Color(temafyne.ColorNameScrollBarBackground, temafyne.VariantLight)},
		{"sombra", SinBarras{}.Color(temafyne.ColorNameShadow, temafyne.VariantDark)},
	} {
		if _, _, _, a := n.c.RGBA(); a != 0 {
			t.Errorf("la %s sigue pintándose (%v) y tiene que ser transparente", n.nombre, n.c)
		}
	}
}

// TestLaBarraDeDesplazamientoEsLaDeLaWeb: `scrollbar-width: thin` y `scrollbar-color:
// var(--sytue-border-strong) transparent` (styles.css). Medido en producción a ×2: el pulgar
// es #839298 —`border-strong` en claro—, de 6 de ancho y con los extremos redondos, sobre
// un carril que no se ve. VISTO EN ROJO sin el tema: la barra de serie, negra al 60 %, de 3
// en reposo y 16 con el ratón encima, sobre un carril gris.
func TestLaBarraDeDesplazamientoEsLaDeLaWeb(t *testing.T) {
	for _, v := range []fyne.ThemeVariant{temafyne.VariantLight, temafyne.VariantDark} {
		if got, quiero := (Sytue{}).Color(temafyne.ColorNameScrollBar, v), Color("border-strong", De(v)); got != color.Color(quiero) {
			t.Errorf("variante %v: la barra es %v y la web la pinta de `border-strong` (%v)", v, got, quiero)
		}
		if _, _, _, a := (Sytue{}).Color(temafyne.ColorNameScrollBarBackground, v).RGBA(); a != 0 {
			t.Errorf("variante %v: el carril se pinta y en la web es transparente", v)
		}
	}
	for n, quiero := range map[fyne.ThemeSizeName]float32{
		temafyne.SizeNameScrollBar: 6, temafyne.SizeNameScrollBarSmall: 6, temafyne.SizeNameScrollBarRadius: 3,
	} {
		if got := (Sytue{}).Size(n); got != quiero {
			t.Errorf("%s mide %v y la web %v", n, got, quiero)
		}
	}
}

// TestElEncimaDeLasListasEsElDeLaWeb: la fila de `widget.List` y el `widget.Slider` pintan
// el hover que les da el tema, y la web pinta `hover:bg-bg-subtle` en esas filas. Medido
// el 2026-09-27 en oscuro: la fila de Mail y la de Drive pasan a #14232a en la web y a
// blanco al 6 % (#ffffff0f) en Fyne. VISTO EN ROJO sin el caso de `ColorNameHover`.
func TestElEncimaDeLasListasEsElDeLaWeb(t *testing.T) {
	for _, v := range []fyne.ThemeVariant{temafyne.VariantLight, temafyne.VariantDark} {
		if got, quiero := (Sytue{}).Color(temafyne.ColorNameHover, v), Color("bg-subtle", De(v)); got != color.Color(quiero) {
			t.Errorf("variante %v: el hover es %v y la web pinta `bg-subtle` (%v)", v, got, quiero)
		}
	}
}
