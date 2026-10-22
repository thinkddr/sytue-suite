// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
)

// atrasEn reparte el «atrás» que llega por el receptor de Android como reparte el driver
// móvil la tecla (typeDownCanvas): al control enfocado; si no hay ninguno, a la ranura de
// teclas de la ventana; y si nadie la tiene, la app se cierra, que es lo que hace Android
// con «atrás» en la primera pantalla.
func atrasEn(a fyne.App, c fyne.Canvas) {
	tecla := &fyne.KeyEvent{Name: mobile.KeyBack}
	switch {
	case c.Focused() != nil:
		c.Focused().TypedKey(tecla)
	case c.OnTypedKey() != nil:
		c.OnTypedKey()(tecla)
	default:
		if d, ok := a.Driver().(interface{ GoBack() }); ok {
			d.GoBack()
		}
	}
}

// El driver móvil enseña la última ventana visible (una presentación puede ir
// delante del editor). CanvasForObject resuelve su lienzo activo: no se manda
// Atrás siempre a la primera ventana de la aplicación.
func atrasEnVentanaActual(a fyne.App) {
	ventanas := a.Driver().AllWindows()
	if len(ventanas) == 0 {
		return
	}
	w := ventanas[len(ventanas)-1]
	c := a.Driver().CanvasForObject(w.Content())
	if c == nil {
		c = w.Canvas()
	}
	atrasEn(a, c)
}
