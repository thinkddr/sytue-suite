// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestLaRejillaSoloMontaLoQueSeVe: con 2.000 elementos la cuadrícula monta las filas
// que se ven (y una más por cada lado), mide como si estuvieran todas, monta las que
// entran al desplazar, y Fin lleva el foco a la ÚLTIMA de verdad, no a la última
// montada. VISTO EN ROJO con la rejilla de antes, que montaba las 2.000.
func TestLaRejillaSoloMontaLoQueSeVe(t *testing.T) {
	tr := traductor(t)
	w := test.NewWindow(nil)
	w.SetPadded(false)
	t.Cleanup(w.Close)
	p := NuevaPantalla(tr, nil, nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	e := p.explor
	e.Rejilla, e.Fase, e.Elementos = true, Exito, elementosDeBanco(2000)
	p.Repintar()
	r := e.rejillaViva
	if r == nil || len(e.tarjetas) == 0 || len(e.tarjetas) > 60 {
		t.Fatalf("montadas %d tarjetas de 2.000", len(e.tarjetas))
	}
	cols := r.rej.Columnas(e.anchoRejilla)
	alto := r.altoTarjeta(r.rej.AnchoColumna(e.anchoRejilla))
	filas := (2000 + cols - 1) / cols
	if got, quiero := r.MinSize(nil).Height, float32(filas)*alto+float32(filas-1)*r.rej.Hueco; got != quiero {
		t.Errorf("la rejilla mide %v y con todas mediría %v", got, quiero)
	}
	// La rueda: es como llega un desplazamiento de verdad (avisa a OnScrolled).
	e.desplazable.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -100*(alto+r.rej.Hueco))})
	if e.tarjetas[0].i > 100*cols || e.tarjetas[len(e.tarjetas)-1].i < 100*cols {
		t.Errorf("tras desplazar a la fila 100 se ven de la %d a la %d", e.tarjetas[0].i, e.tarjetas[len(e.tarjetas)-1].i)
	}
	// Una ventana más alta, con el mismo ancho, monta las filas que ahora se ven.
	antes := e.tarjetas[len(e.tarjetas)-1].i
	w.Resize(fyne.NewSize(1280, 1600))
	if despues := e.tarjetas[len(e.tarjetas)-1].i; despues <= antes {
		t.Errorf("con la ventana más alta la última montada sigue siendo la %d (antes, la %d)", despues, antes)
	}
	primera := e.tarjetas[0]
	w.Canvas().Focus(primera.objeto.(fyne.Focusable))
	primera.t.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	f, _ := w.Canvas().Focused().(fyne.CanvasObject)
	ultima, ok := r.hechas[1999]
	if !ok || f != ultima.objeto {
		t.Fatalf("Fin no lleva a la última tarjeta (montada: %v)", ok)
	}
	ultima.t.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if f, _ := w.Canvas().Focused().(fyne.CanvasObject); f != r.hechas[1999-cols].objeto {
		t.Error("↑ desde la última no lleva a la de encima")
	}
}
