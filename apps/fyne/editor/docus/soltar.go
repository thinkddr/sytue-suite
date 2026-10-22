// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import "fyne.io/fyne/v2"

// Soltar es soltar texto sobre el documento abierto, que en la web hace el navegador en
// el `contenteditable` de la página (y `PageCanvas.tsx` recoge como `insertFromDrop`
// cuando el control de cambios está puesto). `pos` es absoluta en el lienzo, la de
// `soltar.Soltado`. Devuelve false si no hay documento o lo soltado cae fuera de la
// página.
func (a *App) Soltar(texto string, pos fyne.Position) bool {
	return a.abierto != nil && a.abierto.folio != nil && a.abierto.folio.Soltar(texto, pos)
}

// Soltar pone el cursor donde se soltó y escribe ahí el texto, por el mismo camino que
// pegar: el historial, el control de cambios y el guardado lo ven igual.
func (f *Folio) Soltar(texto string, pos fyne.Position) bool {
	d := fyne.CurrentApp().Driver()
	if f.pag == nil || texto == "" || !f.editable() || d.CanvasForObject(f.pag) == nil {
		return false
	}
	a, t := d.AbsolutePositionForObject(f.pag), f.pag.Size()
	if pos.X < a.X || pos.Y < a.Y || pos.X >= a.X+t.Width || pos.Y >= a.Y+t.Height {
		return false
	}
	f.enfocar()
	f.mover(f.enPunto(pos.Subtract(a)), false)
	f.escribir(texto)
	return true
}
