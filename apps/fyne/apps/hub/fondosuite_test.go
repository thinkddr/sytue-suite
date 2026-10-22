// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func TestLasOchoRaicesDelHubUsanYConservanElFondoSuite(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	tr := traductor(t)
	ahora := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	for _, caso := range []struct {
		nombre string
		nuevo  func() fyne.Widget
	}{
		{"lanzador", func() fyne.Widget { return NuevoLanzador(tr, ahora) }},
		{"actividad", func() fyne.Widget { return NuevaActividad(tr, ahora) }},
		{"avisos", func() fyne.Widget { return NuevaAvisos(tr) }},
		{"buscar", func() fyne.Widget { return NuevoBuscar(tr) }},
		{"bienvenida", func() fyne.Widget { return NuevaBienvenida(tr) }},
		{"dispositivos", func() fyne.Widget { return NuevosDispositivos(tr) }},
		{"datos", func() fyne.Widget { return NuevosDatos(tr) }},
		{"descargas", func() fyne.Widget { return NuevasDescargas(tr) }},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			tema.Elegir("claro")
			w := caso.nuevo()
			w.Resize(fyne.NewSize(800, 600))
			r := test.WidgetRenderer(w)
			fondo, ok := r.Objects()[0].(*ui.FondoSuite)
			if !ok {
				t.Fatal("la raíz sigue usando su fondo privado")
			}
			for _, eleccion := range []struct {
				nombre string
				v      tema.Variante
			}{{"traslucido", tema.Translucido}, {"oscuro", tema.Oscuro}, {"claro", tema.Claro}} {
				tema.Elegir(eleccion.nombre)
				w.Refresh()
				if r.Objects()[0] != fondo || fondo.Variante != eleccion.v {
					t.Errorf("%s recrea el lienzo o no aplica la apariencia %s", caso.nombre, eleccion.nombre)
				}
				if tam := fondo.Size(); tam.Width != 800 || tam.Height < 600 {
					t.Errorf("%s deja parte de la pantalla sin lienzo: %v", caso.nombre, tam)
				}
			}
		})
	}
}
