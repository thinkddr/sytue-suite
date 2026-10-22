// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaCasillaNativaSigueAlTema: con `color-scheme: dark` Chromium pinta OTRA casilla.
// Medido en Chromium, píxel del centro y del borde.
func TestLaCasillaNativaSigueAlTema(t *testing.T) {
	g := func(n uint8) color.NRGBA { return color.NRGBA{R: n, G: n, B: n, A: 255} }
	for v, quiero := range map[tema.Variante]ColoresCasilla{
		tema.Claro: {
			Borde: g(0x76), BordeEncima: g(0x4f), Fondo: g(0xff),
			Marcada: color.NRGBA{R: 0x00, G: 0x75, B: 0xff, A: 255}, MarcadaEncima: color.NRGBA{R: 0x00, G: 0x5c, B: 0xc8, A: 255}, Tic: g(0xff),
			BordeInerte: g(0xcf), FondoInerte: g(0xf7), MarcadaInerte: g(0xcf), TicInerte: g(0xeb),
		},
		tema.Oscuro: {
			Borde: g(0x85), BordeEncima: g(0xac), Fondo: g(0x3b),
			Marcada: color.NRGBA{R: 0x99, G: 0xc8, B: 0xff, A: 255}, MarcadaEncima: color.NRGBA{R: 0xd1, G: 0xe6, B: 0xff, A: 255}, Tic: g(0x3b),
			BordeInerte: g(0x62), FondoInerte: g(0x3b), MarcadaInerte: g(0x74), TicInerte: g(0x3b),
		},
	} {
		if got := CasillaNativa(v); got != quiero {
			t.Errorf("variante %v: %+v; Chromium pinta %+v", v, got, quiero)
		}
	}
}

// TestElRangoNativoEsElDeChromium: el `<input type="range">` sin estilo de la cinta de
// CertiPDF, medido en Chromium a 56×16 con el valor en 2 de 1..12: carril de 8 con borde
// de 1 a lo ancho, pulgar de 16 y, en oscuro, sus otros colores.
func TestElRangoNativoEsElDeChromium(t *testing.T) {
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		tema.Elegir(map[tema.Variante]string{tema.Claro: "claro", tema.Oscuro: "oscuro"}[v])
		d := NuevoDeslizador(1, 12, 2, nil)
		d.Nativo = true
		if m := d.MinSize(); m.Height != 16 {
			t.Errorf("el rango nativo mide %g de alto; en Chromium, 16", m.Height)
		}
		img := colocado(t, d, fyne.NewSize(56, 16), 10)
		n := RangoNativo(v)
		en := func(x, y float32) color.NRGBA { return px(img, int((10+x)*escala), int((10+y)*escala)) }
		// A la derecha del pulgar: el carril (centro) y su borde (fila de arriba).
		if got := en(40, 8); !cerca(got, n.Carril) {
			t.Errorf("variante %v: el carril se pinta %v; Chromium, %v", v, got, n.Carril)
		}
		if got := en(40, 4.25); !cerca(got, n.BordeCarril) {
			t.Errorf("variante %v: el borde del carril se pinta %v; Chromium, %v", v, got, n.BordeCarril)
		}
		// El carril empieza 1 px dentro: la primera columna es del fondo.
		if got := en(55.1, 8); cerca(got, n.Carril) || cerca(got, n.BordeCarril) {
			t.Errorf("variante %v: el carril llega al borde derecho de la caja (%v); Chromium lo mete 1 px", v, got)
		}
		// El pulgar, en 3,64 + 8 (fracción 1/11 de los 40 que recorre), arriba del carril.
		if got := en(11.6, 1.5); !cerca(got, n.Acento) {
			t.Errorf("variante %v: el pulgar se pinta %v; Chromium, %v", v, got, n.Acento)
		}
	}
	tema.Elegir("")
}

// TestLaFlechaDelSelectNativoEsDelColorDelTexto: la flecha de Chromium usa
// `currentColor`; en un `<select>` sin tinta propia es la del texto, en los dos temas.
func TestLaFlechaDelSelectNativoEsDelColorDelTexto(t *testing.T) {
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		s := NuevoSelector([]string{"A"}, nil)
		s.Propio = &SelectorPropio{Campo: CampoPropio{Texto: tema.TextoSM, Borde: "border"}, Alto: 28.5}
		_, _, texto, pista := s.colores(v)
		if pista != texto {
			t.Errorf("variante %v: la flecha es %v y el texto %v; en Chromium son el mismo", v, pista, texto)
		}
	}
}
