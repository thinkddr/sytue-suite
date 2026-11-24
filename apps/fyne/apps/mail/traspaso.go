// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

// paridad-web: apps/frontend/apps/mail/src/components/TraspasoBoveda.tsx

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El paso del correo de Matrix a la bóveda (ADR 0087)
//
// Las dos pantallas de `TraspasoBoveda.tsx`, con las mismas claves. Solo piden
// confirmación: lo que hacen (vaciar y pasar, o seguir) lo decide el anfitrión del
// correo privado (cifrado.AplicacionCorreo).

// PasarABoveda es «Pasa tu correo a la nueva bóveda»: para quien tiene el correo
// cifrado con Matrix, pasarlo a la bóveda vaciándolo (con la casilla de que se
// pierde el correo cifrado con el sistema anterior) o seguir por ahora con el
// anterior. Sale antes de intentar abrir Matrix: no hace falta poder abrirlo para
// empezar de nuevo.
type PasarABoveda struct {
	raiz     fyne.CanvasObject
	entiendo *widget.Check
	pasar    *widget.Button
	seguir   *widget.Button
}

// NuevaPasarABoveda la monta. `confirmar` solo se llama con la casilla marcada.
func NuevaPasarABoveda(t *i18n.Translator, confirmar, seguir func()) *PasarABoveda {
	v := &PasarABoveda{}
	titulo := widget.NewLabelWithStyle(t.T("privateMail.vaultMoveTitle", nil), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	cuerpo := widget.NewLabel(t.T("privateMail.vaultMoveBody", nil))
	cuerpo.Wrapping = fyne.TextWrapWord
	v.pasar = widget.NewButton(t.T("privateMail.handoverConfirm", nil), func() {
		if !v.entiendo.Checked {
			return
		}
		v.entiendo.SetChecked(false)
		confirmar()
	})
	v.pasar.Importance = widget.DangerImportance
	v.pasar.Disable()
	v.entiendo = widget.NewCheck(t.T("privateMail.vaultMoveUnderstood", nil), func(marcada bool) { habilitar(v.pasar, marcada) })
	v.seguir = widget.NewButton(t.T("privateMail.vaultMoveKeep", nil), seguir)
	v.raiz = container.NewPadded(container.NewVBox(titulo, cuerpo, v.entiendo, container.NewHBox(v.pasar, v.seguir)))
	return v
}

// Objeto es la pantalla.
func (v *PasarABoveda) Objeto() fyne.CanvasObject { return v.raiz }

// OfertaTraspaso es la salida «¿No puedes abrir tu correo? Vacíalo y pásalo a la
// bóveda» debajo del correo de Matrix mientras no está abierto, con su
// confirmación.
type OfertaTraspaso struct {
	raiz      *fyne.Container
	ofrecer   *widget.Button
	confirmar *fyne.Container
	entiendo  *widget.Check
	vaciar    *widget.Button
	cancelar  *widget.Button
}

// NuevaOfertaTraspaso la monta. `confirmar` solo se llama con la casilla marcada.
func NuevaOfertaTraspaso(t *i18n.Translator, confirmar func()) *OfertaTraspaso {
	o := &OfertaTraspaso{}
	o.ofrecer = widget.NewButton(t.T("privateMail.handoverOffer", nil), func() {
		o.entiendo.SetChecked(false)
		o.ofrecer.Hide()
		o.confirmar.Show()
	})
	cuerpo := widget.NewLabel(t.T("privateMail.handoverBody", nil))
	cuerpo.Wrapping = fyne.TextWrapWord
	o.vaciar = widget.NewButton(t.T("privateMail.handoverConfirm", nil), func() {
		if !o.entiendo.Checked {
			return
		}
		o.cerrar()
		confirmar()
	})
	o.vaciar.Importance = widget.DangerImportance
	o.vaciar.Disable()
	o.entiendo = widget.NewCheck(t.T("privateMail.handoverUnderstood", nil), func(marcada bool) { habilitar(o.vaciar, marcada) })
	o.cancelar = widget.NewButton(t.T("action.cancel", nil), o.cerrar)
	o.confirmar = container.NewVBox(
		widget.NewLabelWithStyle(t.T("privateMail.handoverTitle", nil), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		cuerpo, o.entiendo, container.NewHBox(o.cancelar, o.vaciar),
	)
	o.confirmar.Hide()
	o.raiz = container.NewPadded(container.NewVBox(o.ofrecer, o.confirmar))
	return o
}

// Objeto es la salida.
func (o *OfertaTraspaso) Objeto() fyne.CanvasObject { return o.raiz }

// cerrar esconde la confirmación y desmarca la casilla.
func (o *OfertaTraspaso) cerrar() {
	o.entiendo.SetChecked(false)
	o.confirmar.Hide()
	o.ofrecer.Show()
}

func habilitar(b *widget.Button, si bool) {
	if si {
		b.Enable()
	} else {
		b.Disable()
	}
}
