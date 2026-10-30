// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
)

// Enfocar aplica la política de foco de la suite también desde las apps: en móvil
// sólo los campos de escritura reciben foco y nil lo retira.
func Enfocar(c fyne.Canvas, f fyne.Focusable) { enfocar(c, f) }

// # El foco en el teléfono
//
// En el móvil Fyne abre el teclado en pantalla con CUALQUIER objeto enfocado, no solo
// con un campo (`handleKeyboard` del driver móvil). Los controles de la suite se
// enfocan al pulsarlos —es el `:focus` de la web, y lo que hace que Escape o las flechas
// lleguen al control—, así que en un teléfono tocar un botón, abrir el cajón o un
// diálogo sacaba el teclado encima de la pantalla (visto en Cast, en el emulador, al
// tocar la cámara de «Emitir», 2026-09-28). Un navegador de teléfono no hace eso: el
// teclado sale al tocar donde se escribe.
//
// enfocar es el `Focus` de la suite: en el teléfono solo enfoca lo que escribe (lo que
// implementa `mobile.Keyboardable`, como el editor de los campos). Todo `ui` enfoca por
// aquí; lo vigila TestEnElTelefonoElTecladoSoloSaleParaEscribir.
func enfocar(c fyne.Canvas, f fyne.Focusable) {
	if c == nil {
		return
	}
	if f == nil {
		c.Unfocus() // lo que hace `Focus(nil)`: nadie enfocado
		return
	}
	if esMovil() {
		if _, escribe := f.(mobile.Keyboardable); !escribe {
			return
		}
	}
	c.Focus(f)
}

// EsMovil dice si la app corre en un teléfono: donde se toca con el dedo y no hay
// «pasar por encima». Es lo que en la web deciden `pointer-coarse:` y `hover:`.
func EsMovil() bool { return esMovil() }

// esMovil dice si la app corre en un teléfono. Es una variable por las pruebas: el
// driver de pruebas de Fyne es de escritorio.
var esMovil = func() bool {
	a := fyne.CurrentApp()
	return a != nil && a.Driver().Device().IsMobile()
}
