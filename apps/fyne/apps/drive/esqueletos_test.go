// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestLaListaQueCargaEsUnSoloGrupo: la cabecera de columnas va DENTRO de las filas de
// carga (`ui.FilasEsqueleto` con su cabecera, el `header` de SkeletonRows): un solo
// latido y un solo «Cargando», sin grupos aparte, y sus rayas donde estaban (volcadas
// con la cabecera como `ui.BloquesEsqueleto` propio), también con las columnas que se
// esconden por corte.
//
// VISTO EN ROJO con la cabecera en un grupo aparte: «1 grupos y 0 rayas de cabecera».
func TestLaListaQueCargaEsUnSoloGrupo(t *testing.T) {
	for _, c := range []struct {
		ventana float32
		rayas   []fyne.Position // nil: oculta
	}{
		{1280, []fyne.Position{{X: 24, Y: 10}, {X: 1104, Y: 10}, {X: 1184, Y: 10}}},
		{700, []fyne.Position{{X: 24, Y: 10}, {X: 628, Y: 10}, {}}},
		{500, []fyne.Position{{X: 24, Y: 10}, {}, {}}},
	} {
		e := NuevoExplorador(traductor(t))
		e.Fase, e.Ventana = Cargando, c.ventana
		o := e.esqueletoCuerpo(tema.Claro)
		var filas []*ui.FilasEsqueleto
		grupos := 0
		recorrer(o, func(x fyne.CanvasObject) {
			switch y := x.(type) {
			case *ui.FilasEsqueleto:
				filas = append(filas, y)
			case *ui.BloquesEsqueleto, *ui.Esqueleto:
				grupos++
			}
		})
		if len(filas) != 1 || grupos != 0 || len(filas[0].RayasCabecera()) != 3 {
			n := 0
			if len(filas) > 0 {
				n = len(filas[0].RayasCabecera())
			}
			t.Fatalf("a %g la lista carga con %d filas, %d grupos y %d rayas de cabecera", c.ventana, len(filas), grupos, n)
		}
		w := test.NewTempWindow(t, o)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(c.ventana, 800))
		d := fyne.CurrentApp().Driver()
		for i, r := range filas[0].RayasCabecera() {
			quiere := c.rayas[i]
			if quiere == (fyne.Position{}) {
				if r.Visible() {
					t.Errorf("a %g la raya %d de la cabecera se pinta y la columna está oculta", c.ventana, i)
				}
				continue
			}
			if pos := d.AbsolutePositionForObject(r); pos != quiere || r.Size().Height != 9.6 {
				t.Errorf("a %g la raya %d cae en %v (alto %g); antes, en %v", c.ventana, i, pos, r.Size().Height, quiere)
			}
		}
	}
}

// TestLasCargasDeDriveLatenAUna: los detalles y los miembros de un equipo cargan con UN
// `ui.BloquesEsqueleto` cada uno —un solo latido—, sin `ui.Esqueleto` sueltos, y con la
// MISMA geometría de antes (las cifras salen de volcar sus rectángulos con un esqueleto
// por bloque).
//
// VISTO EN ROJO con los esqueletos sueltos de antes: 16 y 3.
func TestLasCargasDeDriveLatenAUna(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Cargando, 1280
	type bloque struct {
		i        int
		pos, tam fyne.Size
	}
	for _, c := range []struct {
		nombre  string
		o       fyne.CanvasObject
		tam     fyne.Size
		bloques int
		mira    []bloque
	}{
		{"detalles", esqueletoDetalles(320), fyne.NewSize(320, 600), 2 * FilasEsqueletoDetalles, []bloque{
			{1, fyne.NewSize(128, 0), fyne.NewSize(144, 14)},
			{2, fyne.NewSize(0, 26), fyne.NewSize(112, 14)},
		}},
		{"miembros", (&dialogoMiembros{cuerpo: container.NewStack()}).lista(tema.Claro), fyne.NewSize(400, 300), 3, []bloque{
			{0, fyne.NewSize(0, 8), fyne.NewSize(400, 36)},
			{2, fyne.NewSize(0, 96), fyne.NewSize(400, 36)},
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			var grupos []*ui.BloquesEsqueleto
			sueltos := 0
			recorrer(c.o, func(x fyne.CanvasObject) {
				switch y := x.(type) {
				case *ui.BloquesEsqueleto:
					grupos = append(grupos, y)
				case *ui.Esqueleto:
					sueltos++
				}
			})
			if len(grupos) != 1 || len(grupos[0].Bloques()) != c.bloques || sueltos != 0 {
				t.Fatalf("carga con %d grupos y %d esqueletos sueltos", len(grupos), sueltos)
			}
			w := test.NewTempWindow(t, c.o)
			w.SetPadded(false)
			w.Resize(c.tam)
			d := fyne.CurrentApp().Driver()
			for _, m := range c.mira {
				b := grupos[0].Bloques()[m.i]
				if pos, tam := d.AbsolutePositionForObject(b), b.Size(); pos != fyne.NewPos(m.pos.Width, m.pos.Height) || tam != m.tam {
					t.Errorf("el bloque %d cae en %v y mide %v; antes, %v y %v", m.i, pos, tam, m.pos, m.tam)
				}
			}
		})
	}
}
