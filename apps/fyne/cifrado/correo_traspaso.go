// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// vistaReidentificar pide volver a entrar en Sytue ID cuando la bóveda ya está
// abierta pero su clave de firma no es la activa: tras un vaciado o un traspaso,
// Sytue ID pide una identificación reciente. Al volver se termina sin vaciar otra
// vez. Las pantallas del traspaso son de Mail (apps/mail, PasarABoveda y
// OfertaTraspaso) y llevan su catálogo; esta usa solo el común.
type vistaReidentificar struct {
	raiz       fyne.CanvasObject
	entrar     *widget.Button
	reintentar *widget.Button
}

func nuevaVistaReidentificar(t func(string) string, entrar, reintentar func()) *vistaReidentificar {
	v := &vistaReidentificar{}
	titulo := widget.NewLabelWithStyle(t("matrix.title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	cuerpo := widget.NewLabel(t("matrix.signInBody"))
	cuerpo.Wrapping = fyne.TextWrapWord
	v.entrar = widget.NewButton(t("vault.signInAgain"), entrar)
	v.entrar.Importance = widget.HighImportance
	v.reintentar = widget.NewButton(t("action.retry"), reintentar)
	v.raiz = container.NewPadded(container.NewVBox(titulo, cuerpo, container.NewHBox(v.entrar, v.reintentar)))
	return v
}
