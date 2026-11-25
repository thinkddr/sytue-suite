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

// TestLaCabeceraDeLaPaginaVaASangre: «Inicio» va arriba de borde a borde de la columna,
// como la cabecera de Mail o Drive, y las secciones empiezan debajo con su margen. Dentro
// de la caja centrada era una franja blanca suelta sobre el gris de Carril (auditoría del
// 2026-11-24).
func TestLaCabeceraDeLaPaginaVaASangre(t *testing.T) {
	for _, ancho := range []float32{390, 1020, 1600} {
		v := vistaDePrueba(t, tema.Claro)
		v.Ventana = ancho
		p := PantallaInicio(v, Inicio{Mios: misVideos()})
		w := test.NewWindow(p)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(ancho, 900))
		var cab *ui.CabeceraApp
		for _, o := range p.Objects {
			if c, es := o.(*ui.CabeceraApp); es {
				cab = c
			}
		}
		if cab == nil {
			t.Fatalf("a %v la cabecera no es hija directa de la página", ancho)
		}
		if cab.Position() != (fyne.Position{}) || cab.Size().Width != ancho {
			t.Errorf("a %v la cabecera está en %v y mide %v; quiero (0,0) y el ancho entero", ancho, cab.Position(), cab.Size().Width)
		}
		caja := p.Objects[len(p.Objects)-1]
		if y := caja.Position().Y; y != cab.Size().Height+MargenPagina {
			t.Errorf("a %v las secciones empiezan en y=%v; quiero bajo la cabecera y su margen (%v)", ancho, y, cab.Size().Height+MargenPagina)
		}
		w.Close()
	}
}
