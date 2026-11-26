// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestSubirEsElMasFlotanteEnEscritorio: el «Subir» de Cast es UN control, el «+» flotante
// de abajo a la derecha, también en escritorio (producto, 2026-11-25; el guardián web es
// `boton-flotante.test.ts`): la cabecera de Inicio ya no lo repite, y se queda con
// «Emitir».
//
// VISTO EN ROJO con el «Subir» primario de la cabecera de antes y con la capa escondida
// desde 768.
func TestSubirEsElMasFlotanteEnEscritorio(t *testing.T) {
	a, w := appDePrueba(t, servicioCompleto(), true)
	w.Resize(fyne.NewSize(1280, 800))
	a.Arrancar()
	_ = w.Canvas().Capture()
	rotulo := a.v.Traduce("nav.upload")
	var fab *ui.BotonFlotante
	recorrer(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.BotonFlotante:
			fab = x
		case *ui.Boton:
			if x.Texto == rotulo && x.Visible() {
				t.Errorf("Inicio sigue con su botón «%s»: crear es el «+» flotante", rotulo)
			}
		}
	})
	if fab == nil || !fab.Visible() || fab.Nombre != rotulo {
		t.Fatal("a 1280 no está el «+» flotante de «Subir»")
	}
	if pos := fab.Position(); pos.X+fab.Size().Width != 1280-ui.MargenFlotanteEn(1280) {
		t.Errorf("el «+» está en %v y va en la esquina de abajo a la derecha", pos)
	}
}
