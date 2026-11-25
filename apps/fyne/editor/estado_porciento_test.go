// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestElPorcentajeDelZoomMideSuTexto: del 75 % al 100 % el porcentaje de la barra de
// estado se quedaba con la caja de dos cifras y se leía «10…»: repintar la barra no la
// volvía a colocar.
func TestElPorcentajeDelZoomMideSuTexto(t *testing.T) {
	ctx := &Contexto{Tipo: Documento, Zoom: 75, Estado: Listo}
	b := NuevaBarraEstado(ctx)
	w := test.NewTempWindow(t, b)
	w.Resize(fyne.NewSize(1280, AltoEstado))
	ctx.Zoom = 100
	b.Refresh()
	if got, quiero := b.porciento.Size().Width, ui.MedidaEnlace(b.porciento).Width; got < quiero {
		t.Errorf("«%s» mide %v y su texto %v: se recorta", b.porciento.Texto, got, quiero)
	}
}
