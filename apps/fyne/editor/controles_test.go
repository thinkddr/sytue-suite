// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// cajaDe devuelve el rectángulo de píxeles de dispositivo donde aparece un color.
func cajaDe(img image.Image, c color.NRGBA) (x0, x1, y0, y1 int) {
	b := img.Bounds()
	x0, x1, y0, y1 = 1<<30, -1, 1<<30, -1
	for y := range b.Dy() {
		for x := range b.Dx() {
			if cerca(px(img, x, y), c) {
				x0, x1, y0, y1 = min(x0, x), max(x1, x), min(y0, y), max(y1, y)
			}
		}
	}
	return x0, x1, y0, y1
}

// TestElSelectorDeLaCintaMideLoDeRibbonSelect: `h-7` (28 con la raíz de 16) y el
// ancho que se le pida, 120 si no; el borde es `border` y pinta de borde a borde.
func TestElSelectorDeLaCintaMideLoDeRibbonSelect(t *testing.T) {
	s := NuevoSelectorCinta("Tema", 128, []Opcion{{"", "Papel"}, {"carbon", "Carbón"}}, "carbon", nil)
	if got := s.MinSize(); got != fyne.NewSize(128, 28) {
		t.Errorf("mide %v y RibbonSelect es 128 × 28", got)
	}
	if got := MedidaSelectorCinta(0); got.Width != 120 {
		t.Errorf("sin ancho mide %v y el `width ?? 120` de la web da 120", got.Width)
	}
	s.Resize(s.MinSize())
	s.Move(fyne.NewPos(10, 10))
	img := pinta(t, 160, 50, s)
	x0, x1, y0, y1 := cajaDe(img, tema.Color("border", tema.Claro))
	ancho, alto := float32(x1-x0+1)/escala, float32(y1-y0+1)/escala
	if ancho < 127.5 || ancho > 128.5 || alto < 27.5 || alto > 28.5 {
		t.Errorf("el borde pinta %v × %v y son 128 × 28", ancho, alto)
	}
}

// TestElSelectorDeLaCintaEligeConLasFlechas: las flechas de un `<select>` cerrado
// cambian el valor y avisan, sin salirse de las opciones.
func TestElSelectorDeLaCintaEligeConLasFlechas(t *testing.T) {
	var elegidos []string
	s := NuevoSelectorCinta("Transición", 118, []Opcion{{"none", "Sin"}, {"fade", "Fundido"}}, "none",
		func(v string) { elegidos = append(elegidos, v) })
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if len(elegidos) != 2 || elegidos[0] != "fade" || elegidos[1] != "none" {
		t.Errorf("con ↓ ↓ ↑ ha elegido %v; tocaba [fade none]", elegidos)
	}
}

// TestLaMuestraDeColorLlevaSuBarrita: la barrita de 16 × 3 del color actual, a 4 del
// borde de abajo y centrada; y 'transparent' se pinta con el color del borde para que
// no desaparezca.
func TestLaMuestraDeColorLlevaSuBarrita(t *testing.T) {
	m := NuevaMuestraCinta(nil, iconos.EdImagen, "Fondo", "#DC2626", nil, nil)
	m.Resize(MedidaBotonCinta())
	img := pinta(t, 28, 28, m)
	rojo, _ := Hex("#DC2626")
	x0, x1, y0, y1 := cajaDe(img, rojo)
	if ancho, alto := float32(x1-x0+1)/escala, float32(y1-y0+1)/escala; ancho != 16 || alto != 3 {
		t.Errorf("la barrita pinta %v × %v y es `w-4 h-[3px]`: 16 × 3", ancho, alto)
	}
	if abajo := 28 - float32(y1+1)/escala; abajo != 4 {
		t.Errorf("la barrita queda a %v del borde de abajo y `bottom-1` son 4", abajo)
	}
}

// TestLaPaletaEsLaRejillaDeLaWeb: seis columnas de 24 con 4 de hueco, y al pulsar una
// muestra se elige ese color y la paleta se recoge.
//
// VISTO EN ROJO (2026-09-22, en el frontal): con `grid-cols-6` escrito como 4 la
// paleta medía 91 de ancho y el test lo decía. Deshecho.
func TestLaPaletaEsLaRejillaDeLaWeb(t *testing.T) {
	if got := MedidaPaleta(len(PaletaContenido)); got != fyne.NewSize(6*24+5*4, 2*24+4) {
		t.Errorf("la paleta de doce mide %v y son seis columnas de 24 con 4 de hueco", got)
	}
	var elegido string
	ctx := &Contexto{}
	m := NuevaMuestraCinta(ctx, iconos.EdImagen, "Fondo", "transparent", nil, func(c string) { elegido = c })
	w := test.NewTempWindow(t, container.NewWithoutLayout(m))
	w.Resize(fyne.NewSize(400, 300))
	m.Resize(MedidaBotonCinta())
	test.Tap(m)
	if len(w.Canvas().Overlays().List()) != 1 {
		t.Fatal("pulsar la muestra tiene que abrir la paleta")
	}
	test.Tap(m.paleta.celdas[8].(*celdaPaleta))
	if elegido != PaletaContenido[8] || m.Color != PaletaContenido[8] || len(w.Canvas().Overlays().List()) != 0 {
		t.Errorf("al elegir el azul: elegido %q, actual %q, %d capas abiertas", elegido, m.Color,
			len(w.Canvas().Overlays().List()))
	}
}

// TestHexLeeLosColoresDeContenido: los doce de la paleta se leen, y 'transparent' no.
func TestHexLeeLosColoresDeContenido(t *testing.T) {
	for _, c := range PaletaContenido {
		if _, ok := Hex(c); !ok {
			t.Errorf("%s no se lee", c)
		}
	}
	if _, ok := Hex("transparent"); ok {
		t.Error("'transparent' no es un color que pintar")
	}
}

// TestElSelectorSinSuValorQuedaEnBlanco: un `<select>` controlado con un valor que no
// está entre sus opciones queda en blanco (`selectedIndex = -1`), no enseña la primera.
// Lo pidió Diapos: su selector de fuente enseñaba «Inter» con una fuente de fuera.
func TestElSelectorSinSuValorQuedaEnBlanco(t *testing.T) {
	s := NuevoSelectorCinta("Fuente", 128, []Opcion{{"Inter", "Inter"}, {"Arial", "Arial"}}, "Comic Sans", nil)
	if got := s.rotuloActual(); got != "" {
		t.Errorf("con un valor de fuera enseña %q", got)
	}
	if s.Valor = "Arial"; s.rotuloActual() != "Arial" {
		t.Errorf("con un valor de la lista enseña %q", s.rotuloActual())
	}
}
