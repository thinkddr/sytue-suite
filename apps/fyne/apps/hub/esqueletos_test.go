// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// cargasDe son los grupos de carga y los `ui.Esqueleto` sueltos que pinta un widget.
func cargasDe(o fyne.CanvasObject) (grupos []*ui.BloquesEsqueleto, sueltos int) {
	var andar func(fyne.CanvasObject)
	andar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.BloquesEsqueleto:
			grupos = append(grupos, x)
		case *ui.Esqueleto:
			sueltos++
		case *fyne.Container:
			for _, h := range x.Objects {
				andar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				andar(h)
			}
		}
	}
	andar(o)
	return grupos, sueltos
}

// TestLasCargasDelHubLatenAUna: «Hoy», «Reciente», la bandeja, sus reglas y la
// actividad cargan con UN `ui.BloquesEsqueleto` cada una —un solo latido y un solo
// «Cargando», con el `recent.loading` y el `inbox.loading` de la web donde los hay—,
// sin `ui.Esqueleto` sueltos, y con la MISMA geometría de antes (las cifras salen de
// volcar sus rectángulos con un esqueleto por bloque).
//
// VISTO EN ROJO con los esqueletos sueltos de antes: 2 y 16 en la portada, 12 en la
// bandeja, 2 por origen en las reglas y 5 en la actividad.
func TestLasCargasDelHubLatenAUna(t *testing.T) {
	tr := traductor(t)
	l := lanzadorDePrueba(t)
	l.Reciente.Estado, l.Agenda.Estado, l.Reciente.Items = Cargando, Cargando, nil
	a := NuevaAvisos(tr)
	a.Estado = Cargando
	reglas := NuevaAvisos(tr)
	reglas.Estado, reglas.CargaReglas = Listo, ReglasCargando
	origenes := 0
	for _, g := range reglas.gruposVisibles() {
		origenes += len(g.Origenes)
	}
	ac := NuevaActividad(tr, time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC))
	ac.Estado = Cargando
	type bloque struct {
		grupo, i int
		pos, tam fyne.Size
	}
	for _, c := range []struct {
		nombre  string
		o       fyne.CanvasObject
		alto    float32
		bloques []int
		rotulos []string
		mira    []bloque
	}{
		{"portada", l, 1600, []int{2, 16}, []string{"", tr.T("recent.loading", nil)}, []bloque{
			{0, 0, fyne.NewSize(41, 814.375), fyne.NewSize(252.45937, 16)},
			{0, 1, fyne.NewSize(41, 838.375), fyne.NewSize(198.36095, 16)},
			{1, 0, fyne.NewSize(475.65625, 828.375), fyne.NewSize(10, 10)},
			{1, 3, fyne.NewSize(1175, 826.375), fyne.NewSize(48, 12)},
		}},
		{"bandeja", a, 1600, []int{3 * FilasEsqueletoAvisos}, []string{tr.T("inbox.loading", nil)}, []bloque{
			{0, 0, fyne.NewSize(28, 226), fyne.NewSize(18, 18)},
			{0, 1, fyne.NewSize(58, 218), fyne.NewSize(537.3, 14)},
			{0, 2, fyne.NewSize(58, 240), fyne.NewSize(835.8, 12)},
		}},
		{"reglas", reglas, 3000, []int{2 * origenes}, []string{""}, []bloque{
			{0, 0, fyne.NewSize(24, 493), fyne.NewSize(36, 20)},
			{0, 1, fyne.NewSize(70, 496), fyne.NewSize(251.09999, 14)},
		}},
		{"actividad", ac, 1600, []int{5}, []string{""}, []bloque{
			{0, 0, fyne.NewSize(24, 150), fyne.NewSize(1232, 58)},
			{0, 1, fyne.NewSize(24, 212), fyne.NewSize(1232, 58)},
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			w := test.NewTempWindow(t, c.o)
			w.SetPadded(false)
			w.Resize(fyne.NewSize(1280, c.alto))
			grupos, sueltos := cargasDe(c.o)
			if len(grupos) != len(c.bloques) || sueltos != 0 {
				t.Fatalf("carga con %d grupos y %d esqueletos sueltos", len(grupos), sueltos)
			}
			for i, g := range grupos {
				if len(g.Bloques()) != c.bloques[i] {
					t.Errorf("el grupo %d tiene %d bloques y son %d", i, len(g.Bloques()), c.bloques[i])
				}
				if r := c.rotulos[i]; r != "" && g.AccessibilityLabel() != r {
					t.Errorf("el grupo %d se anuncia «%s» y es «%s»", i, g.AccessibilityLabel(), r)
				}
			}
			d := fyne.CurrentApp().Driver()
			for _, m := range c.mira {
				b := grupos[m.grupo].Bloques()[m.i]
				if pos, tam := d.AbsolutePositionForObject(b), b.Size(); pos != fyne.NewPos(m.pos.Width, m.pos.Height) || tam != m.tam {
					t.Errorf("el bloque %d del grupo %d cae en %v y mide %v; antes, %v y %v", m.i, m.grupo, pos, tam, m.pos, m.tam)
				}
			}
		})
	}
}

// TestLasFilasDeCargaDeRecienteSeVen: las barras de cada fila de «Reciente» miden su
// fracción de la columna de texto y no pisan la meta de la derecha.
//
// VISTO EN ROJO con los `ui.Esqueleto` de antes, que nadie redimensionaba: las tres
// barras de cada fila medían 0 × 0 y la fila de carga era solo el punto.
func TestLasFilasDeCargaDeRecienteSeVen(t *testing.T) {
	l := lanzadorDePrueba(t)
	l.Reciente.Estado, l.Reciente.Items = Cargando, nil
	w := test.NewTempWindow(t, l)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 1600))
	r := test.WidgetRenderer(l).(*lanzadorRenderer)
	for i := 0; i+3 < len(r.esqRec); i += 4 {
		meta := r.esqRec[i+3].(*canvas.Rectangle)
		for _, b := range r.esqRec[i+1 : i+3] {
			if b.Size().Width <= 0 || b.Size().Height <= 0 {
				t.Errorf("fila %d: una barra mide %v", i/4, b.Size())
			}
			if fin := b.Position().X + b.Size().Width; fin+HuecoReciente > meta.Position().X {
				t.Errorf("fila %d: una barra acaba en %v y pisa el hueco de la meta, que empieza en %v", i/4, fin, meta.Position().X)
			}
		}
		if meta.Size().Width <= 0 {
			t.Errorf("fila %d: la meta mide %v", i/4, meta.Size())
		}
	}
}
