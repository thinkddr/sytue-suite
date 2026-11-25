// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// TestElBuscadorDelCarrilEnfocadoSigueSiendoUnVelo: en el teléfono (y en Marco a
// cualquier ancho) el buscador de la suite va sobre la barra de color, con el relleno en
// velo translúcido (`sidebar-field`). Enfocado, el aro RELLENO de siempre se veía entero
// a través del velo y el campo quedaba como una píldora maciza del color de foco, con el
// texto a ~1,3:1 (auditoría del 2026-11-24). El aro va abierto: dentro del campo sigue el
// velo sobre la barra, el anillo está FUERA, y lo escrito contrasta con lo de detrás.
//
// VISTO EN ROJO con el aro relleno: dentro del campo pinta el `sidebar-focus`.
// Con `SYTUE_CAPTURAS=/dir` deja la barra enfocada en claro y en oscuro.
func TestElBuscadorDelCarrilEnfocadoSigueSiendoUnVelo(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, modo := range []string{"claro", "oscuro"} {
		tema.Elegir(modo)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		const ancho = 390
		b := barraPruebaA(t, ancho)
		c := b.buscador.campo
		if !c.EnCarril {
			t.Fatalf("%s: a %v el buscador no va en el carril", modo, ancho)
		}
		c.FocusGained()
		img := pinta(t, ancho, AltoBarraSuite(), b)
		guardarEn(t, os.Getenv("SYTUE_CAPTURAS"), "buscador-carril-foco-"+modo+".png", img)
		p, tam := c.Position(), c.Size()
		fondo := tema.Color(fondoEsquina, v)
		velo := tokens.SobreSRGB(tema.Color("sidebar-field", v), fondo)
		// Dentro, arriba y en el medio: lejos de la lupa y de la marca de agua.
		if got := px(img, int((p.X+tam.Width/2)*escala), int((p.Y+3)*escala)); !cercaDe(got, velo, 3) {
			t.Errorf("%s: enfocado, el campo pinta %v por dentro y es el velo %v: el aro lo rellena", modo, got, velo)
		}
		// Fuera, a un píxel del borde de arriba: el anillo.
		if got, aro := px(img, int((p.X+tam.Width/2)*escala), int((p.Y-1)*escala)), tema.Color("sidebar-focus", v); !cercaDe(got, aro, 3) {
			t.Errorf("%s: enfocado, fuera del campo pinta %v y el aro es %v", modo, got, aro)
		}
		_, _, texto, _ := colorCampoCarril(v, false, false, false)
		if k := contraste(texto, velo); k < 4.5 {
			t.Errorf("%s: lo escrito queda a %.2f:1 del campo enfocado", modo, k)
		}
		c.FocusLost()
	}
}
