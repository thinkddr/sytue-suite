// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Soltar es soltar texto sobre el redactor abierto: entra en el asunto o en el cuerpo, por
// donde cae, como en el `input` y el `contenteditable` de la web, que lo hacen solos. Sin
// redactor, o fuera de esos dos campos, no hace nada y devuelve false. `pos` es absoluta
// en el lienzo, la de `soltar.Soltado`.
//
// ponytail: los destinatarios (`CampoDirecciones`) no son un `widget.Entry` y no reciben
// lo soltado; en la web, soltar una dirección ahí la escribe. Se hace si alguien lo pide.
func (b *Buzon) Soltar(texto string, pos fyne.Position) bool {
	rd := b.redaccion
	if rd == nil || rd.r == nil {
		return false
	}
	// El cuerpo pasado su tope se desplaza: lo que cae fuera de lo que se VE no es suyo.
	a, t := fyne.CurrentApp().Driver().AbsolutePositionForObject(rd.r.cuerpo), rd.r.cuerpo.Size()
	enCuerpo := pos.X >= a.X && pos.Y >= a.Y && pos.X < a.X+t.Width && pos.Y < a.Y+t.Height
	zona, ok := rd.r.rico.Zona().(fyne.CanvasObject)
	return (enCuerpo && ok && ui.SoltarEnCampo(zona, texto, pos)) || ui.SoltarEnCampo(rd.r.asunto, texto, pos)
}
