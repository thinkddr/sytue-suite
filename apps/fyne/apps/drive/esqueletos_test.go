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

// TestLasCargasDeDriveLatenAUna: la cabecera de la lista, los detalles y los miembros
// de un equipo cargan con UN `ui.BloquesEsqueleto` cada uno —un solo latido—, sin
// `ui.Esqueleto` sueltos, y con la MISMA geometría de antes (las cifras salen de volcar
// sus rectángulos con un esqueleto por bloque).
//
// VISTO EN ROJO con los esqueletos sueltos de antes: 3, 16 y 3.
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
		{"cabecera de la lista", e.esqueletoCuerpo(tema.Claro), fyne.NewSize(1280, 800), 3, []bloque{
			{0, fyne.NewSize(24, 10), fyne.NewSize(56, 9.6)},
			{1, fyne.NewSize(1104, 10), fyne.NewSize(48, 9.6)},
			{2, fyne.NewSize(1184, 10), fyne.NewSize(72, 9.6)},
		}},
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
