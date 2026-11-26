// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/local"
)

// TestLosTextosDeCertiPDFEstanEnTodosLosIdiomas: los textos propios de CertiPDF tienen
// las MISMAS claves en los 26 idiomas (PLAN §5.14, C-12), con los huecos del inglés.
func TestLosTextosDeCertiPDFEstanEnTodosLosIdiomas(t *testing.T) {
	for _, f := range (local.Propios{Base: textos, Traducciones: traducciones}).Faltan() {
		t.Error(f)
	}
	if propio("de", "iaEnviar") != "Senden" || propio("es-MX", "iaEnviar") != "Enviar" || propio("xx", "iaEnviar") != "Send" {
		t.Error("propio no elige el idioma")
	}
}
