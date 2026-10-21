// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"testing"

	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestLaCasillaDeTareaSigueAlTema: sin marcar es la de Chromium de ese tema (en oscuro,
// borde #858585 sobre #3b3b3b), no el gris claro sobre `surface`.
func TestLaCasillaDeTareaSigueAlTema(t *testing.T) {
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		o := casillaNativa(Adorno{An: LadoCasillaNativa, Al: LadoCasillaNativa}, v)
		borde, dentro := o[0].(*canvas.Rectangle), o[1].(*canvas.Rectangle)
		if quiero := ui.CasillaNativa(v); borde.FillColor != quiero.Borde || dentro.FillColor != quiero.Fondo {
			t.Errorf("variante %v: borde %v y fondo %v; Chromium, %v y %v", v, borde.FillColor, dentro.FillColor, quiero.Borde, quiero.Fondo)
		}
	}
}
