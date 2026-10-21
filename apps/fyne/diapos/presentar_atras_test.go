// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
)

func TestAtrasVuelveDelModeradorYDespuesSaleDeLaPresentacion(t *testing.T) {
	antes := cadaSegundo
	cadaSegundo = func(func()) func() { return func() {} }
	t.Cleanup(func() { cadaSegundo = antes })
	p := NuevaPresentacion(leeModelo(t), 0, traductor(t))
	w := test.NewTempWindow(t, p)
	w.Resize(fyne.NewSize(390, 844))
	salidas := 0
	p.AlSalir = func() { salidas++ }
	p.AlCambiarModerador = ModeradorEnLaMisma(w, p, "es")
	teclado(w, p)
	p.Runa('p')
	w.Resize(fyne.NewSize(844, 390))
	w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack})
	if p.Moderador || w.Content() != p || salidas != 0 {
		t.Fatalf("atrás desde el moderador deja moderador=%v, contenido=%T, salidas=%d", p.Moderador, w.Content(), salidas)
	}
	w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack})
	if salidas != 1 {
		t.Fatalf("atrás desde la presentación pidió salir %d veces", salidas)
	}
}
