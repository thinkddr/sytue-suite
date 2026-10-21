// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestLaBarraDeOpcionesTraslucidaOcultaLaFoto(t *testing.T) {
	tema.Elegir("traslucido")
	t.Cleanup(func() { tema.Elegir("") })
	v := vistaDePrueba(t, nil)
	v.Variante = tema.Translucido
	barra := fondosFlotantes(v, Editor{Lienzo: NuevoLienzoEditor()})
	tam := barra.MinSize()
	barra.Resize(tam)
	if caja, _ := buscarDesenfocado(barra, fyne.Position{}); caja != nil {
		t.Error("la placa opaca conserva un desenfoque que no puede verse")
	}
	for _, fondo := range []color.NRGBA{{R: 255, A: 255}, {B: 255, A: 255}} {
		debajo := canvas.NewRectangle(fondo)
		debajo.Resize(tam)
		img := pinta(t, tam.Width, tam.Height, debajo, barra)
		// El centro del relleno inferior queda fuera de rótulos y muestras, y
		// también lejos de los arcos de la píldora y de su sombra.
		if got, want := px(img, int(tam.Width/2*escala), int((tam.Height-3)*escala)), tema.Color("menu-surface", tema.Translucido); !cerca(got, want) {
			t.Errorf("las opciones sobre la foto %v pintan %v; el papel opaco es %v", fondo, got, want)
		}
	}
}
