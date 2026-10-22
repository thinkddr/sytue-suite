// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
)

func TestElAtrasDelTelefonoGuardaAntesDeVolver(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Hola</p>"}
	a, f := abrirParaEscribir(t, s)
	w := test.NewWindow(a.Objeto())
	defer func() { w.Close(); fyne.DoAndWait(func() {}) }()
	a.Arrancar()
	f.escribir("!")
	w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack})
	if a.abierto != nil || a.raiz.Objects[0] != a.portada.Objeto() {
		t.Fatal("atrás no vuelve a la portada")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ops) != 1 || !strings.Contains(s.ops[0], "Hola!") {
		t.Fatalf("lo pendiente no se guarda antes de volver: %v", s.ops)
	}
}
