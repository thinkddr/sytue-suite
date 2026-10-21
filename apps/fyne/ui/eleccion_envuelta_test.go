// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

// Chromium, JSX real de ProcesoObligacion/Form, Figtree y rem16, 2026-10-04:
// etiqueta natural 320.140625x22; etiqueta móvil 254x44 dentro del label278.
func TestLaEleccionReparteSuEtiquetaSinCambiarElControl(t *testing.T) {
	const rotulo = "He revisado las cifras y los documentos necesarios."
	e := NuevaCasilla(rotulo, nil)
	for _, c := range []struct{ limite, ancho, alto float32 }{
		{0, 344.140625, 22}, {646, 344.140625, 22}, {254, 278, 44},
	} {
		e.AnchoEtiqueta = c.limite
		m := e.MinSize()
		if math.Abs(float64(m.Width-c.ancho)) > .3 || m.Height != c.alto {
			t.Errorf("etiqueta a %.0f: %v; Chromium %.6fx%.0f", c.limite, m, c.ancho, c.alto)
		}
		r := e.CreateRenderer().(*eleccionRenderer)
		e.Resize(m)
		r.Layout(m)
		if c.limite > 0 {
			if !r.parrafo.Visible() || r.texto.Visible() || r.parrafo.Position().X != 24 || r.parrafo.Size().Height != c.alto {
				t.Fatal("el párrafo debe sustituir la etiqueta simple y empezar a 24 px")
			}
		} else if !r.texto.Visible() || r.parrafo.Visible() {
			t.Fatal("el comportamiento natural sigue usando la etiqueta simple")
		}
	}
	if e.AccessibilityLabel() != rotulo {
		t.Fatal("el nombre accesible conserva toda la etiqueta")
	}
	cambios := 0
	e.AlCambiar = func(bool) { cambios++ }
	e.Tapped(&fyne.PointEvent{Position: fyne.NewPos(270, 40)})
	if !e.Marcada || cambios != 1 {
		t.Fatal("pulsar la segunda línea marca la misma casilla")
	}
	e.Disable()
	e.TypedRune(' ')
	if !e.Marcada || cambios != 1 {
		t.Fatal("el reparto no altera disabled ni teclado")
	}
}

func TestLaEleccionEnvueltaMideElAnchoAsignadoAlRedimensionar(t *testing.T) {
	e := NuevaCasilla("Incluir en la búsqueda de la suite", nil)
	e.AnchoEtiqueta = 500
	col := container.New(Columna{}, e)
	for _, ancho := range []float32{400, 180, 400} {
		h := AltoA(col, ancho)
		col.Resize(fyne.NewSize(ancho, h))
		r := test.WidgetRenderer(e).(*eleccionRenderer)
		if r.parrafo.Position().Y < 0 || r.parrafo.Position().Y+r.parrafo.Size().Height > e.Size().Height+0.01 {
			t.Fatalf("la etiqueta sale de la casilla a %v: párrafo=%v/%v, control=%v", ancho, r.parrafo.Position(), r.parrafo.Size(), e.Size())
		}
		if ancho == 180 && e.Size().Height <= e.escalon().Linea {
			t.Fatal("el padre conserva una sola línea al encoger")
		}
	}
}
