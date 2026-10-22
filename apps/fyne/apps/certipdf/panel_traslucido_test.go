// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestElPanelTraslucidoOcultaElDocumento(t *testing.T) {
	tema.Elegir("traslucido")
	t.Cleanup(func() { tema.Elegir("") })
	tam := fyne.NewSize(AnchoPanel, 400)
	for _, caso := range []struct {
		nombre string
		y      float32
		crear  func() fyne.CanvasObject
	}{
		{"panel", 300, func() fyne.CanvasObject {
			return NuevoPanelLateral(traductorES(t), &Contenido{}).Objeto()
		}},
		{"cabecera del cajón", 5, func() fyne.CanvasObject {
			p := NuevaPantalla(traductorES(t))
			p.panel = NuevoPanelLateral(traductorES(t), &Contenido{})
			return p.cajon()[2]
		}},
	} {
		panel := caso.crear()
		panel.Resize(tam)
		for _, fondo := range []color.NRGBA{{R: 255, A: 255}, {B: 255, A: 255}} {
			debajo := canvas.NewRectangle(fondo)
			debajo.Resize(tam)
			w := test.NewWindow(container.NewWithoutLayout(debajo, panel))
			w.SetPadded(false)
			w.Resize(tam)
			img := w.Canvas().Capture()
			escala := float32(img.Bounds().Dx()) / tam.Width
			got := px(img, int(100*escala), int(caso.y*escala))
			if want := tema.Color("menu-surface", tema.Translucido); got != want {
				t.Errorf("el PDF %v se mezcla con %s: %v; el papel opaco es %v", fondo, caso.nombre, got, want)
			}
			w.Close()
		}
	}
}
