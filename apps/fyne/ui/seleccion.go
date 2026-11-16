// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// La selección múltiple de una lista o una galería: un toque marca y desmarca, y Mayús +
// clic marca el TRAMO desde la última marcada sin Mayús (el ancla). Es `idsDelRango` y
// `marcarConTramo` de @sytue/ui (packages/ui/src/lib/seleccion.ts); vive aquí para que
// Drive, Correo y Photos cuenten igual.

// Mayus dice si la tecla Mayús está pulsada ahora mismo. Un toque de Fyne no trae los
// modificadores, pero el driver de escritorio sabe cuáles hay pulsados; en móvil no hay.
func Mayus() bool {
	app := fyne.CurrentApp()
	if app == nil {
		return false
	}
	d, ok := app.Driver().(desktop.Driver)
	return ok && d.CurrentKeyModifiers()&fyne.KeyModifierShift != 0
}

// MarcarRango marca los elementos entre dos índices, ambos incluidos, en cualquier orden,
// sin desmarcar nada. Los que no tienen id (`id` devuelve "") se saltan.
func MarcarRango[T any](sel map[string]bool, els []T, id func(T) string, a, b int) {
	lo, hi := min(a, b), max(a, b)
	for i := max(lo, 0); i <= hi && i < len(els); i++ {
		if k := id(els[i]); k != "" {
			sel[k] = true
		}
	}
}

// MarcarConTramo es un toque sobre el elemento `indice` de `els` y devuelve el ancla
// nueva. Sin Mayús (o sin ancla) conmuta ese y lo deja de ancla; con Mayús AÑADE el
// tramo del ancla a él sin desmarcar nada ni mover el ancla, para que un segundo Mayús +
// clic corrija el tramo desde el mismo sitio.
func MarcarConTramo[T any](sel map[string]bool, els []T, id func(T) string, indice, ancla int, mayus bool) int {
	if mayus && ancla >= 0 && indice >= 0 {
		MarcarRango(sel, els, id, ancla, indice)
		return ancla
	}
	if indice < 0 || indice >= len(els) {
		return ancla
	}
	k := id(els[indice])
	if k == "" {
		return ancla
	}
	if sel[k] {
		delete(sel, k)
	} else {
		sel[k] = true
	}
	return indice
}
