// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"math"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

type lienzoSeguroNotes struct {
	fyne.Canvas
	tam fyne.Size
}

func (c *lienzoSeguroNotes) InteractiveArea() (fyne.Position, fyne.Size) {
	return fyne.Position{}, c.tam
}

type ventanaSeguraNotes struct {
	fyne.Window
	lienzo fyne.Canvas
}

func (w ventanaSeguraNotes) Canvas() fyne.Canvas { return w.lienzo }

func TestLaRotacionDeNotesMideLaHojaDentroDelAreaSegura(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	original := "carril"
	if tema.EsMarco() {
		original = "marco"
	}
	t.Cleanup(func() { tema.ElegirForma(original) })
	tema.ElegirForma("marco")
	p, w := montarCaptura(t, 390, 844, "list")
	c := &lienzoSeguroNotes{Canvas: w.Canvas(), tam: fyne.NewSize(390, 844)}
	p.win = ventanaSeguraNotes{Window: w, lienzo: c}
	for _, caso := range []struct{ ventana, segura fyne.Size }{
		{fyne.NewSize(390, 844), fyne.NewSize(390, 763)},
		{fyne.NewSize(844, 390), fyne.NewSize(756, 369)},
		{fyne.NewSize(390, 844), fyne.NewSize(390, 763)},
	} {
		c.tam = caso.segura
		w.Resize(caso.ventana)
		p.raiz.Resize(caso.segura)
		p.Repintar()
		if p.anchoVentana() != caso.segura.Width {
			t.Fatalf("a %v mide el lienzo completo, no el área útil: %g", caso.ventana, p.anchoVentana())
		}
		arm := p.raiz.Objects[0].(*ui.Armazon)
		if diferencia := math.Abs(float64(p.anchoContenido() - arm.Contenido.Size().Width)); diferencia > 0.01 {
			t.Fatalf("a %v las notas se miden fuera de su hoja: medición=%g, hoja=%v", caso.ventana, p.anchoContenido(), arm.Contenido.Size())
		}
	}
}
