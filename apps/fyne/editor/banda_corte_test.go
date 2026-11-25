// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestLaBandaConMasAcabaEnUnControlEntero: con «Más», la banda acaba tras el último
// control que cabe entero y ninguno se queda a medias pegado a «Más». La galería de
// estilos de Docus dejaba un retal de su cuarta ficha (auditoría del 2026-11-24).
func TestLaBandaConMasAcabaEnUnControlEntero(t *testing.T) {
	defer ui.TactilDePrueba(false)()
	for _, ancho := range []float32{1280, 1100, 900} {
		c, _ := cintaAncha(t, ancho)
		if !c.mas.Visible() {
			t.Fatalf("a %v no sale «Más»", ancho)
		}
		fin := c.banda.Size().Width
		if fin > ancho-c.anchoZonaMas()+0.5 {
			t.Errorf("a %v la banda llega a %v y «Más» empieza en %v", ancho, fin, ancho-c.anchoZonaMas())
		}
		var ir func(o fyne.CanvasObject, x float32)
		ir = func(o fyne.CanvasObject, x float32) {
			x += o.Position().X
			switch h := o.(type) {
			case *fyne.Container:
				for _, n := range h.Objects {
					ir(n, x)
				}
				return
			case *Grupo:
				for _, n := range h.Controles {
					ir(n, x)
				}
				return
			}
			if _, ok := o.(fyne.Tappable); ok && x < fin && x+o.Size().Width > fin+0.5 {
				t.Errorf("a %v un control va de %v a %v y la banda acaba en %v: se ve a medias", ancho, x, x+o.Size().Width, fin)
			}
		}
		for _, o := range objetosDeBanda(c.banda.Contenido) {
			ir(o, c.banda.Contenido.Position().X)
		}
	}
}
