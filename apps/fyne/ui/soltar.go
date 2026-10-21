// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// SoltarEnCampo es soltar texto sobre un campo, que el navegador hace él solo en un
// `input`, un `textarea` o un `contenteditable`: el cursor va adonde se soltó y el texto
// entra ahí como si se pegara. `pos` es absoluta en el lienzo, la de `soltar.Soltado`.
// Devuelve false si `pos` cae fuera de `campo`, si el campo no se ve o está apagado, o si
// no es un campo que se escriba.
//
// Pega con un portapapeles propio y no con el del sistema: soltar no toca lo que la
// persona tuviera copiado.
func SoltarEnCampo(campo fyne.CanvasObject, texto string, pos fyne.Position) bool {
	sc, ok := campo.(fyne.Shortcutable)
	if !ok || texto == "" || !campo.Visible() {
		return false
	}
	if d, ok := campo.(fyne.Disableable); ok && d.Disabled() {
		return false
	}
	a, t := fyne.CurrentApp().Driver().AbsolutePositionForObject(campo), campo.Size()
	if pos.X < a.X || pos.Y < a.Y || pos.X >= a.X+t.Width || pos.Y >= a.Y+t.Height {
		return false
	}
	// El clic que pone ahí el cursor y el foco, como el `drop` del navegador.
	if m, ok := campo.(desktop.Mouseable); ok {
		e := &desktop.MouseEvent{
			PointEvent: fyne.PointEvent{Position: pos.Subtract(a), AbsolutePosition: pos},
			Button:     desktop.MouseButtonPrimary,
		}
		m.MouseDown(e)
		m.MouseUp(e)
	}
	sc.TypedShortcut(&fyne.ShortcutPaste{Clipboard: &portapapelesSoltado{texto}})
	return true
}

// portapapelesSoltado es el portapapeles de un solo uso con lo soltado.
type portapapelesSoltado struct{ texto string }

func (p *portapapelesSoltado) Content() string     { return p.texto }
func (p *portapapelesSoltado) SetContent(s string) { p.texto = s }
