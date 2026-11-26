// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/local"
)

// TestLosTextosDeContactsEstanEnTodosLosIdiomas: los textos propios de Contacts tienen
// las MISMAS claves en los 26 idiomas (PLAN §5.14, C-12), con los huecos del inglés en
// su orden; y el aviso de conflicto nombra la copia como la llama «sync.copia».
func TestLosTextosDeContactsEstanEnTodosLosIdiomas(t *testing.T) {
	for _, f := range (local.Propios{Base: textos, Traducciones: traducciones}).Faltan() {
		t.Error(f)
	}
	for loc, tr := range traducciones {
		if !strings.Contains(tr["sync.conflicto"], tr["sync.copia"]) {
			t.Errorf("%s: «sync.conflicto» no cita %q", loc, tr["sync.copia"])
		}
	}
}
