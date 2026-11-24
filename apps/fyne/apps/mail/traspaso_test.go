// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// «Pasa tu correo a la nueva bóveda»: vaciar solo con la casilla marcada (y la
// casilla vuelve a quedar sin marcar), y seguir con el anterior sin casilla.
func TestPasarABovedaPideLaCasillaAntesDeVaciar(t *testing.T) {
	test.NewTempApp(t)
	tr := traductor(t)
	confirmados, seguidos := 0, 0
	v := NuevaPasarABoveda(tr, func() { confirmados++ }, func() { seguidos++ })
	if v.Objeto() == nil || !v.pasar.Disabled() {
		t.Fatal("«vaciar y pasar» encendido sin confirmar")
	}
	if v.pasar.Text != tr.T("privateMail.handoverConfirm", nil) || v.pasar.Text == "privateMail.handoverConfirm" || v.seguir.Text == "privateMail.vaultMoveKeep" {
		t.Fatalf("las claves no salen del catálogo de Mail: %q %q", v.pasar.Text, v.seguir.Text)
	}
	test.Tap(v.pasar)
	if confirmados != 0 {
		t.Fatal("se vació sin la casilla")
	}
	v.entiendo.SetChecked(true)
	if v.pasar.Disabled() {
		t.Fatal("con la casilla marcada no se puede vaciar")
	}
	test.Tap(v.pasar)
	if confirmados != 1 || v.entiendo.Checked || !v.pasar.Disabled() {
		t.Fatal("vaciar no confirmó una vez, o la casilla siguió marcada")
	}
	test.Tap(v.seguir)
	if seguidos != 1 || confirmados != 1 {
		t.Fatal("seguir con el anterior no siguió")
	}
}

// La salida debajo del correo de Matrix: se abre, pide la casilla, y cancelar la
// cierra sin vaciar.
func TestOfertaTraspasoConfirmaConCasilla(t *testing.T) {
	test.NewTempApp(t)
	tr := traductor(t)
	confirmados := 0
	o := NuevaOfertaTraspaso(tr, func() { confirmados++ })
	if o.Objeto() == nil || o.confirmar.Visible() || !o.ofrecer.Visible() || o.ofrecer.Text == "privateMail.handoverOffer" {
		t.Fatal("la salida no empieza cerrada, o su texto no sale del catálogo")
	}
	test.Tap(o.ofrecer)
	if !o.confirmar.Visible() || o.ofrecer.Visible() || !o.vaciar.Disabled() {
		t.Fatal("ofrecer no abre la confirmación con «vaciar» apagado")
	}
	test.Tap(o.vaciar)
	o.entiendo.SetChecked(true)
	test.Tap(o.cancelar)
	if confirmados != 0 || o.confirmar.Visible() || o.entiendo.Checked {
		t.Fatal("cancelar vació o dejó la casilla marcada")
	}
	test.Tap(o.ofrecer)
	o.entiendo.SetChecked(true)
	test.Tap(o.vaciar)
	if confirmados != 1 || o.confirmar.Visible() {
		t.Fatal("confirmar no vació una vez")
	}
}

// Con Matrix retirada (ADR 0087, entrega 5): sin «seguir con el anterior», con el
// texto de la retirada antes de la casilla, y vaciar solo con la casilla.
func TestPasarABovedaConMatrixRetiradaNoOfreceSeguir(t *testing.T) {
	test.NewTempApp(t)
	tr := traductor(t)
	confirmados := 0
	v := NuevaPasarABoveda(tr, func() { confirmados++ }, nil)
	if v.seguir != nil {
		t.Fatal("con Matrix retirada se ofrece seguir con el anterior")
	}
	if v.cuerpo.Text != tr.T("privateMail.vaultMoveRetiredBody", nil) || v.cuerpo.Text == "privateMail.vaultMoveRetiredBody" {
		t.Fatalf("el texto no es el de la retirada del catálogo: %q", v.cuerpo.Text)
	}
	caja := v.raiz.(*fyne.Container).Objects[0].(*fyne.Container)
	if len(caja.Objects) != 4 || caja.Objects[1] != v.cuerpo || caja.Objects[2] != v.entiendo {
		t.Fatal("el aviso de la retirada no va antes de la casilla")
	}
	if titulo := caja.Objects[0].(*widget.Label).Text; titulo != tr.T("privateMail.vaultMoveRetiredTitle", nil) {
		t.Fatalf("el título no es el de la retirada: %q", titulo)
	}
	if botones := caja.Objects[3].(*fyne.Container).Objects; len(botones) != 1 || botones[0] != v.pasar {
		t.Fatal("con Matrix retirada hay más botones que «vaciar y pasar»")
	}
	test.Tap(v.pasar)
	v.entiendo.SetChecked(true)
	test.Tap(v.pasar)
	if confirmados != 1 {
		t.Fatal("vaciar con la casilla no confirmó una vez")
	}
}

// La duda con Matrix retirada: el aviso del catálogo y «Reintentar».
func TestCorreoSinComprobarReintenta(t *testing.T) {
	test.NewTempApp(t)
	tr := traductor(t)
	veces := 0
	v := NuevaCorreoSinComprobar(tr, func() { veces++ })
	if v.Objeto() == nil || v.reintentar.Text != tr.T("common.retry", nil) || v.reintentar.Text == "common.retry" {
		t.Fatalf("el botón no sale del catálogo: %q", v.reintentar.Text)
	}
	test.Tap(v.reintentar)
	if veces != 1 {
		t.Fatal("«Reintentar» no reintentó")
	}
}
