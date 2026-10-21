// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"math"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Chromium, Table/PartesInforme reales, Figtree14px con tnum (2026-10-04).
// El objeto comparte la geometría de CSS incluso al recibir otra caja exterior.
func TestElTextoTabularComoObjetoConservaLaCajaDeLinea(t *testing.T) {
	for _, c := range []struct {
		texto string
		ancho float32
	}{{"1", 8.734375}, {"3025,00 €", 68.234375}} {
		tabular := NuevoTextoTabular(c.texto, tema.TextoBase, Normal, tema.Color("text", tema.Claro))
		o := tabular.Objeto()
		m := o.MinSize()
		if math.Abs(float64(m.Width-c.ancho)) > .1 || m.Height != 22 {
			t.Errorf("%q: %v, Chromium%.6fx22", c.texto, m, c.ancho)
		}
		o.Resize(fyne.NewSize(200, 40))
		for _, letra := range o.Objects {
			if letra.Position().Y < 0 || letra.Position().Y+letra.MinSize().Height > 22.3 {
				t.Errorf("glifo fuera de la caja de línea: %v", letra.Position())
			}
		}
	}
}
