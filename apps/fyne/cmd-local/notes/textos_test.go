// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import "testing"

// TestLaTablaDeNotepadsCuadra: los textos propios de Notepads están en los 26 idiomas
// (C-12), sin claves de más ni de menos.
func TestLaTablaDeNotepadsCuadra(t *testing.T) {
	for _, f := range tabla.Faltan() {
		t.Error(f)
	}
	if got := tabla.En("de", "sin.calendar"); got != "Sytue Calendar von Sytue Office ist auf diesem Gerät nicht installiert." {
		t.Errorf("alemán: %q", got)
	}
}
