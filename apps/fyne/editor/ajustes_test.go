// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// TestElProductoSustituyeElPanelAjustes: con `PanelAjustes` el cuerpo del panel derecho es
// el del producto (Docus); sin él, o si devuelve nil, el genérico de alineación (Calcus,
// Diapos).
func TestElProductoSustituyeElPanelAjustes(t *testing.T) {
	propio := widget.NewLabel("ajustes de Docus")
	c := &Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, Ajustes: true}
	c.PanelAjustes = func() fyne.CanvasObject { return propio }
	if got := cuerpoAjustes(c); got != propio {
		t.Errorf("el cuerpo de «Ajustes» no es el del producto: %T", got)
	}
	c.PanelAjustes = func() fyne.CanvasObject { return nil }
	if got := cuerpoAjustes(c); got == propio || got == nil {
		t.Error("sin cuerpo del producto queda el genérico")
	}
}
