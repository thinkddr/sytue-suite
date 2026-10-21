// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// gifDePrueba son tres fotogramas de 4×1: rojo entero, luego un píxel verde encima con
// disposición «fondo» y luego uno azul en otro sitio.
func gifDePrueba() *gif.GIF {
	pal := color.Palette{color.Transparent, color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 255, 0, 255}, color.NRGBA{0, 0, 255, 255}}
	rojo := image.NewPaletted(image.Rect(0, 0, 4, 1), pal)
	for x := range 4 {
		rojo.SetColorIndex(x, 0, 1)
	}
	verde := image.NewPaletted(image.Rect(1, 0, 2, 1), pal)
	verde.SetColorIndex(1, 0, 2)
	azul := image.NewPaletted(image.Rect(3, 0, 4, 1), pal)
	azul.SetColorIndex(3, 0, 3)
	return &gif.GIF{
		Image:    []*image.Paletted{rojo, verde, azul},
		Delay:    []int{0, 2, 2},
		Disposal: []byte{gif.DisposalNone, gif.DisposalBackground, gif.DisposalNone},
		Config:   image.Config{Width: 4, Height: 1},
	}
}

// Los fotogramas salen COMPUESTOS: el segundo lleva el rojo de debajo, y el tercero ya
// no lleva el verde, que se dispuso a fondo. Un retardo de 0 son 100 ms, como en el
// navegador.
//
// VISTO EN ROJO (2026-09-25, en el frontal, saboteado a mano): sin el `case
// DisposalBackground`, el tercer fotograma conserva el verde.
func TestElGifSeComponeConSuDisposicion(t *testing.T) {
	fs, ps := ComponerGif(gifDePrueba())
	if len(fs) != 3 || ps[0] != PausaMinimaGif || ps[1] != 20*time.Millisecond {
		t.Fatalf("%d fotogramas, pausas %v", len(fs), ps)
	}
	rgba := func(i, x int) color.NRGBA { return color.NRGBAModel.Convert(fs[i].At(x, 0)).(color.NRGBA) }
	if c := rgba(1, 0); c.R != 255 {
		t.Errorf("el segundo fotograma perdió el rojo de debajo: %v", c)
	}
	if c := rgba(1, 1); c.G != 255 {
		t.Errorf("el segundo fotograma no lleva su verde: %v", c)
	}
	if c := rgba(2, 1); c.A != 0 {
		t.Errorf("el tercero conserva el verde dispuesto a fondo: %v", c)
	}
	if c := rgba(2, 3); c.B != 255 {
		t.Errorf("el tercero no lleva su azul: %v", c)
	}
}

// En pantalla avanza; con «menos movimiento» se queda en el primero; y al destruir el
// renderer el reloj se para.
//
// VISTO EN ROJO (2026-09-25, en el frontal): quitando `!SinMovimiento` de `animado`, el
// GIF quieto avanza.
func TestElGifSeMueveSoloCuandoToca(t *testing.T) {
	w := NuevoGif(gifDePrueba())
	win := test.NewTempWindow(t, w)
	win.Resize(fyne.NewSize(40, 10))
	avanza := func() bool {
		for range 50 {
			time.Sleep(10 * time.Millisecond)
			var f int
			fyne.DoAndWait(func() { f = w.Fotograma() })
			if f != 0 {
				return true
			}
		}
		return false
	}
	if !avanza() {
		t.Error("el GIF en pantalla no pasa del primer fotograma")
	}
	test.WidgetRenderer(w).Destroy()
	if w.parar != nil {
		t.Error("destruido el renderer, el reloj sigue")
	}

	antes := SinMovimiento
	SinMovimiento = true
	defer func() { SinMovimiento = antes }()
	quieto := NuevoGif(gifDePrueba())
	test.NewTempWindow(t, quieto)
	w = quieto
	if avanza() {
		t.Error("con SinMovimiento el GIF avanza")
	}
	if quieto.parar != nil {
		t.Error("con SinMovimiento hay reloj")
	}
}
