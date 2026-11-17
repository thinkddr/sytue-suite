// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestLasCargasDeCastLatenAUna: la rejilla, las calidades, «Mis directos» y el envío
// desde Drive cargan con UN `ui.BloquesEsqueleto` cada una —un solo latido y un solo
// «Cargando»—, sin `ui.Esqueleto` sueltos, y con la MISMA geometría de antes (las
// cifras salen de volcar sus rectángulos con un esqueleto por bloque).
//
// VISTO EN ROJO con los esqueletos sueltos de antes: 16, 4, 2 y 4.
func TestLasCargasDeCastLatenAUna(t *testing.T) {
	type bloque struct {
		i        int
		pos, tam fyne.Size
	}
	for _, c := range []struct {
		nombre  string
		ventana float32
		hacer   func(Vista) fyne.CanvasObject
		bloques int
		mira    []bloque
	}{
		{"rejilla", 1200, func(v Vista) fyne.CanvasObject { return Esqueletos(v, 4) }, 16, []bloque{
			{0, fyne.NewSize(0, 0), fyne.NewSize(389.33334, 219)},
			{1, fyne.NewSize(0, 233.4375), fyne.NewSize(311.46667, 13)},
			{3, fyne.NewSize(0, 290.75), fyne.NewSize(233.60002, 12)},
		}},
		{"calidades", 600, func(v Vista) fyne.CanvasObject { return SelectorCalidades(v, SelCalidades{}) }, 4, []bloque{
			{0, fyne.NewSize(0, 43.59375), fyne.NewSize(600, 40)},
			{3, fyne.NewSize(0, 147.59375), fyne.NewSize(600, 20)},
		}},
		{"directos", 1200, func(v Vista) fyne.CanvasObject { return tarjetaDirectos(v, Emitir{}) }, 2, []bloque{
			{0, fyne.NewSize(20, 56), fyne.NewSize(1160, 40)},
			{1, fyne.NewSize(20, 108), fyne.NewSize(1160, 40)},
		}},
		{"drive", 1200, esqueletoDrive, 4, []bloque{
			{1, fyne.NewSize(20, 56), fyne.NewSize(816, 36)},
			{3, fyne.NewSize(880, 0), fyne.NewSize(320, 256)},
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			v := vistaDePrueba(t, tema.Claro)
			v.Ventana = c.ventana
			o := c.hacer(v)
			var grupos []*ui.BloquesEsqueleto
			sueltos := 0
			recorrer(o, func(x fyne.CanvasObject) {
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
			w := test.NewTempWindow(t, o)
			w.SetPadded(false)
			w.Resize(fyne.NewSize(c.ventana, 1400))
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
