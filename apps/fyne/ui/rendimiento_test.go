// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// BenchmarkBarraSuiteMontar es lo que hace cada una de las catorce apps al arrancar:
// crear la barra de la suite, montarla alrededor del contenido y colocarla en la
// ventana de escritorio.
func BenchmarkBarraSuiteMontar(b *testing.B) {
	tr, err := i18n.New(i18n.Default, i18n.Common())
	if err != nil {
		b.Fatal(err)
	}
	w := test.NewWindow(nil)
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	b.ReportAllocs()
	for b.Loop() {
		// En frío, como al arrancar: ninguna sombra está aún en la caché del proceso.
		sombras.vaciar()
		barra := NuevaBarraSuite(tr, nil, iconos.Mail)
		w.SetContent(barra.Montar(canvas.NewRectangle(nil)))
		barra.Parar()
	}
}

// BenchmarkSombra es lo que cuesta la sombra de un panel nuevo: entera (lo que se hacía
// siempre), de nueve partes sin nada en la caché y sacada de la caché.
func BenchmarkSombra(b *testing.B) {
	for _, c := range []struct {
		nombre string
		nivel  tema.NivelSombra
		tam    fyne.Size
	}{
		{"tarjeta-sm", tema.SombraSM, fyne.NewSize(280, 132)},
		{"panel-md", tema.SombraMD, fyne.NewSize(600, 400)},
		{"dialogo-xl", tema.SombraXL, fyne.NewSize(1000, 700)},
	} {
		capas := tema.Sombra(c.nivel, tema.Claro)
		b.Run(c.nombre+"/entera", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				pintarSombra(capas, tema.RadioLG, c.tam, false)
			}
		})
		b.Run(c.nombre+"/nueve-partes", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sombras.vaciar()
				imagenSombra(capas, tema.RadioLG, c.tam, false)
			}
		})
		b.Run(c.nombre+"/cache", func(b *testing.B) {
			sombras.vaciar()
			b.ReportAllocs()
			for b.Loop() {
				s := NuevaSombra(c.nivel, tema.RadioLG)
				s.Ajustar(fyne.Position{}, c.tam, tema.Claro)
			}
		})
	}
}
