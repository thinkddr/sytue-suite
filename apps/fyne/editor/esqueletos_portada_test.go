// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestLaPortadaYLaPapeleraCarganConUnGrupo: las teselas de la portada y las filas de la
// papelera, mientras cargan, son UN `ui.BloquesEsqueleto` —un latido y un «Cargando»—
// con el `rounded-lg` de lo que van a ser, y ningún `ui.Esqueleto` suelto.
//
// VISTO EN ROJO con un `ui.Esqueleto` por bloque: «0 grupos».
func TestLaPortadaYLaPapeleraCarganConUnGrupo(t *testing.T) {
	p := NuevaPortada(Documento, nil, "es", nil)
	p.Estado = Cargando
	x := NuevaPapeleraDocs(Hoja, nil, "es", nil)
	x.Estado = Cargando
	for nombre, c := range map[string]struct {
		o       fyne.CanvasObject
		bloques int
	}{
		"portada":  {p.Objeto(), EsqueletosPortada},
		"papelera": {x.Objeto(), EsqueletosPapelera},
	} {
		w := test.NewTempWindow(t, c.o)
		w.Resize(fyne.NewSize(1280, 900))
		var grupos []*ui.BloquesEsqueleto
		sueltos := 0
		var andar func(fyne.CanvasObject)
		andar = func(o fyne.CanvasObject) {
			switch y := o.(type) {
			case *ui.BloquesEsqueleto:
				grupos = append(grupos, y)
			case *ui.Esqueleto:
				sueltos++
			case *fyne.Container:
				for _, h := range y.Objects {
					andar(h)
				}
			case fyne.Widget:
				for _, h := range test.WidgetRenderer(y).Objects() {
					andar(h)
				}
			}
		}
		andar(c.o)
		if len(grupos) != 1 || len(grupos[0].Bloques()) != c.bloques || sueltos != 0 {
			t.Errorf("%s carga con %d grupos y %d esqueletos sueltos", nombre, len(grupos), sueltos)
			continue
		}
		for _, b := range grupos[0].Bloques() {
			if b.CornerRadius != tema.RadioLG || b.Size().Width <= 0 {
				t.Errorf("%s: un bloque mide %v con radio %g", nombre, b.Size(), b.CornerRadius)
			}
		}
	}
}
