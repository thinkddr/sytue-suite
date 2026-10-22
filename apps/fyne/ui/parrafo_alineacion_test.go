// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElParrafoCentradoEnvuelveComoLaWeb: Abrir.tsx real, medido en Chromium
// a 360 px con CSS1Compat, Figtree cargada y raíz de 16. El p text-sm ocupa
// 260×36; su texto tiene x=74,92188 dentro del p que empieza en 50.
func TestElParrafoCentradoEnvuelveComoLaWeb(t *testing.T) {
	p := NuevoParrafo("Suelta aquí el fichero, o elígelo de tu ordenador.", tema.TextoSM, Normal, "text-muted", tema.Claro).A(260)
	p.Alineacion = fyne.TextAlignCenter
	p.Resize(fyne.NewSize(260, 36))
	r := test.WidgetRenderer(p).(*parrafoRenderer)
	r.Layout(p.Size())
	if len(r.lineas) != 2 || p.MinSize().Height != 36 {
		t.Fatalf("el párrafo tiene %d líneas y mínimo %v; la web dos y alto 36", len(r.lineas), p.MinSize())
	}
	if got := r.lineas[0].Position().X; got-24.92188 > 0.2 || 24.92188-got > 0.2 {
		t.Errorf("la primera línea empieza en %g; la web, 24,92188 dentro de su párrafo", got)
	}
}
