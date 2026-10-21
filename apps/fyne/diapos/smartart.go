// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"math"

	"git.sytue.eu/sytue/one/apps/fyne/editor/figuras"
)

// El diagrama SmartArt: la geometría y el SVG están en `editor/figuras`, compartidos
// con los documentos; aquí, cómo se leen los elementos de una diapositiva.

var disposicionesSmart = figuras.DisposicionesSmart

// geometriaSmart es `smartArtLayout` sobre los elementos de la diapositiva.
func geometriaSmart(disp string, elems []Elemento, vw, vh float64) ([]figuras.NodoSmart, []figuras.ConectorSmart) {
	niveles := make([]int, len(elems))
	for i, e := range elems {
		if e.Nivel != nil {
			niveles[i] = max(0, int(math.Floor(*e.Nivel)))
		}
	}
	return figuras.GeometriaSmart(disp, niveles, vw, vh)
}
