// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// enPapel pone el cromo «papel» con la forma Marco, como lo arranca cmd/calendar, y lo
// deja como estaba al acabar.
func enPapel(t *testing.T) {
	t.Helper()
	antes := tema.EsPapel()
	formaAntes := "marco"
	if !tema.EsMarco() {
		formaAntes = "carril"
	}
	tema.ElegirCromo(tema.CromoPapel)
	tema.ElegirForma("marco")
	t.Cleanup(func() {
		if !antes {
			tema.ElegirCromo("")
		}
		tema.ElegirForma(formaAntes)
	})
}

// croma es la distancia entre el canal más alto y el más bajo: 0 en un gris.
func croma(img image.Image, x, y int) int {
	c := px(img, x, y)
	return int(max(c.R, c.G, c.B)) - int(min(c.R, c.G, c.B))
}

// TestLosGlifosDeLaCabeceraSonDeTinta: las flechas y la lupa son `HEADER_ICON_BTN`,
// `text-text`, y sobre papel de una sola tinta. Salían del azul del fantasma
// (`primary-text`) y, como esa tinta es «de color», la lupa se rellenaba de azul.
//
// VISTO EN ROJO (2026-11-06): con BotonIconoCabecera sobre `ui.Boton` fantasma, la
// flecha y la lupa pintan píxeles de croma 90+ en un botón que debería ser gris.
func TestLosGlifosDeLaCabeceraSonDeTinta(t *testing.T) {
	enPapel(t)
	for _, i := range []iconos.Icono{iconos.ChevronIzquierda, iconos.Buscar} {
		b := BotonIconoCabecera(i, "x", nil)
		w := test.NewWindow(b)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(LadoBotonIcono, LadoBotonIcono))
		img := w.Canvas().Capture()
		w.Close()
		peor, tinta := 0, false
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				peor = max(peor, croma(img, x, y))
				if c := px(img, x, y); c.R < 128 {
					tinta = true
				}
			}
		}
		if !tinta {
			t.Errorf("icono %v: no hay glifo pintado", i)
		}
		// Las tintas neutras de la suite no pasan de 17 (ver `tintaDeColor` en iconos).
		if peor > 20 {
			t.Errorf("icono %v: hay un píxel de croma %d; el glifo es `text-text`, gris", i, peor)
		}
	}
}

// TestLaCortinaDeRecargaEsDelColorDeLaHoja: el `opacity-60` de la web deja ver lo que
// hay detrás de la rejilla, que en Marco es la hoja (`--sytue-sheet-bg`) y no el fondo
// de la página. Sobre papel, el velo de `bg` agrisaba la rejilla blanca al recargar.
//
// VISTO EN ROJO (2026-11-06): con `tema.Color("bg", v)` el velo sale (241,246,246) y la
// hoja es blanca.
func TestLaCortinaDeRecargaEsDelColorDeLaHoja(t *testing.T) {
	enPapel(t)
	c := nuevaCortina()
	test.WidgetRenderer(c).Refresh()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	quiero := tema.Color("sheet-bg", v)
	got, _ := c.velo.FillColor.(color.NRGBA)
	if got.R != quiero.R || got.G != quiero.G || got.B != quiero.B {
		t.Errorf("el velo es %v; detrás de la rejilla está la hoja, %v", got, quiero)
	}
}
