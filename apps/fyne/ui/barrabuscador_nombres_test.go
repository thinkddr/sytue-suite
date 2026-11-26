// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// TestElBuscadorDeLaBarraSeNombraComoEnLaWeb es el combobox de GlobalSearch.tsx para el
// lector de pantalla, en lo que Fyne deja decir (un nombre y un papel): el campo se llama
// «Búsqueda global», su `aria-label`, y no la marca de agua; y cada resultado, por lo que
// se ve de él, título y detalle. VISTO EN ROJO sin el `Rotulo` del campo (decía «Buscar en
// Sytue One…») y con el nombre de la fila solo de título.
func TestElBuscadorDeLaBarraSeNombraComoEnLaWeb(t *testing.T) {
	b, parar := BarraSuiteDePrueba(traductorEs(t), iconos.Drive)
	t.Cleanup(parar)
	if n := b.buscador.campo.AccessibilityLabel(); n != "Búsqueda global" {
		t.Errorf("el buscador se llama %q para el lector; en la web, «Búsqueda global»", n)
	}
	if n := nuevaFilaResultado("Informe anual", "PDF", nil).AccessibilityLabel(); n != "Informe anual, PDF" {
		t.Errorf("un resultado con detalle se llama %q y quiero «Informe anual, PDF»", n)
	}
	if n := nuevaFilaResultado("Informe anual", "", nil).AccessibilityLabel(); n != "Informe anual" {
		t.Errorf("un resultado sin detalle se llama %q y quiero «Informe anual»", n)
	}
}
