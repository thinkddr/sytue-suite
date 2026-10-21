// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// Fuera de macOS Fyne pinta el menú principal DENTRO de la ventana, encima de la barra de
// la suite. Solo va en la barra del sistema de macOS.
func TestElMenuPrincipalSoloEnMacOS(t *testing.T) {
	test.NewTempApp(t)
	m := fyne.NewMenu("Apps", fyne.NewMenuItem("Inicio", func() {}))
	for goos, quiero := range map[string]bool{"darwin": true, "linux": false, "windows": false, "freebsd": false} {
		w := test.NewWindow(nil)
		ponerMenu(w, goos, m)
		if tiene := w.MainMenu() != nil; tiene != quiero {
			t.Errorf("%s: menú principal = %v, quiero %v", goos, tiene, quiero)
		}
		w.Close()
	}
}
