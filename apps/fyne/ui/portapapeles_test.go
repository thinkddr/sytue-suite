// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// portapapelesRoto es uno que no guarda nada: lo que pasa cuando GLFW falla y el driver
// se traga el error.
type portapapelesRoto struct{}

func (portapapelesRoto) Content() string   { return "lo de antes" }
func (portapapelesRoto) SetContent(string) {}

// TestCopiarSabeSiHaLlegado: con un portapapeles que funciona dice que sí, y con uno que
// se traga el valor dice que no. Sin portapapeles, tampoco.
//
// Visto en ROJO a mano (en el frontal): devolviendo siempre `true` —que es lo que se
// hacía antes, sin mirar—, el caso roto sale copiado.
func TestCopiarSabeSiHaLlegado(t *testing.T) {
	if !Copiar(test.NewClipboard(), "imap.sytue.eu") {
		t.Error("un portapapeles que funciona tiene que decir que se ha copiado")
	}
	if Copiar(portapapelesRoto{}, "imap.sytue.eu") {
		t.Error("un portapapeles que no guarda nada no puede decir que se ha copiado")
	}
	if Copiar(nil, "x") {
		t.Error("sin portapapeles no se ha copiado nada")
	}
}
