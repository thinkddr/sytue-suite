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

// Chromium, Onboarding/Switch reales, Figtree y rem16 (2026-10-04):
// etiqueta natural315.21875x22 y móvil266x44; carril36x20 y hueco10.
func TestElConmutadorReparteLaEtiquetaSinAlterarElCarril(t *testing.T) {
	const rotulo = "IVA mensual (gran empresa o devolución mensual)"
	c := NuevoConmutador(rotulo, nil)
	for _, caso := range []struct{ limite, ancho, alto float32 }{{0, 361.21875, 22}, {458, 361.21875, 22}, {266, 312, 44}} {
		c.AnchoEtiqueta = caso.limite
		m := c.MinSize()
		if math.Abs(float64(m.Width-caso.ancho)) > .3 || m.Height != caso.alto {
			t.Errorf("etiqueta a%.0f: %v; Chromium%.6fx%.0f", caso.limite, m, caso.ancho, caso.alto)
		}
		r := c.CreateRenderer().(*conmutadorRenderer)
		c.Resize(m)
		r.Layout(m)
		if r.carril.Size() != fyne.NewSize(36, 20) || r.carril.Position().Y != (caso.alto-20)/2 {
			t.Fatal("el carril mantiene36x20 y se centra en la etiqueta")
		}
		if caso.limite > 0 {
			if !r.parrafo.Visible() || r.texto.Visible() || r.parrafo.Position().X != 46 || r.parrafo.Size().Height != caso.alto {
				t.Fatal("el párrafo sustituye al texto simple desde x46")
			}
		} else if !r.texto.Visible() || r.parrafo.Visible() {
			t.Fatal("cero conserva la etiqueta natural")
		}
	}
	cambios := 0
	c.AlCambiar = func(bool) { cambios++ }
	c.Tapped(&fyne.PointEvent{Position: fyne.NewPos(300, 40)})
	if !c.Encendido || cambios != 1 || c.AccessibilityLabel() != rotulo {
		t.Fatal("la segunda línea conserva acción y nombre accesible")
	}
	c.Disable()
	c.TypedRune(' ')
	if !c.Encendido || cambios != 1 {
		t.Fatal("disabled también protege la etiqueta envuelta")
	}
}

func TestElConmutadorEnvueltoMideElAnchoAsignadoAlRedimensionar(t *testing.T) {
	c := NuevoConmutador("IVA mensual (gran empresa o devolución mensual)", nil)
	c.AnchoEtiqueta = 500
	col := container.New(Columna{}, c)
	for _, ancho := range []float32{400, 180, 400} {
		h := AltoA(col, ancho)
		col.Resize(fyne.NewSize(ancho, h))
		r := test.WidgetRenderer(c).(*conmutadorRenderer)
		if r.parrafo.Position().Y < 0 || r.parrafo.Position().Y+r.parrafo.Size().Height > c.Size().Height+0.01 {
			t.Fatalf("la etiqueta sale del conmutador a %v: párrafo=%v/%v, control=%v", ancho, r.parrafo.Position(), r.parrafo.Size(), c.Size())
		}
		if ancho == 180 && c.Size().Height <= c.escalon().Linea {
			t.Fatal("el padre conserva una sola línea al encoger")
		}
	}
}
