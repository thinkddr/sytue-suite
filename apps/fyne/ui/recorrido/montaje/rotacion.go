// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package montaje adapta los fixtures de app al montaje real de la ventana nativa.
// Está separado de recorrido para que ese paquete no dependa de ui.
package montaje

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// ComprobarRotacion monta temporalmente BarraSuite como cmd/<app>/main.go, gira
// el fixture y lo devuelve al recorrido con su contenido y tamaño originales.
func ComprobarRotacion(t testing.TB, w fyne.Window, nombre string, producto iconos.Producto, capturar bool, preparacion ...func()) {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	contenido, tamano, relleno := w.Canvas().Content(), w.Canvas().Size(), w.Padded()
	barra, restaurar := ui.BarraSuiteDePrueba(tr, producto)
	switch producto {
	case iconos.Docs, iconos.Sheets, iconos.Slides, iconos.PDF:
		barra.ConHojaASangre()
	default:
	}
	defer func() {
		fallo := recover()
		restaurar()
		if fallo != nil {
			panic(fallo) // No reentrar en la configuración que causó el error.
		}
		w.SetContent(contenido)
		w.SetPadded(relleno)
		w.Resize(tamano)
		fyne.CurrentApp().Settings().SetTheme(fyne.CurrentApp().Settings().Theme())
		contenido.Refresh()
		fyne.DoAndWait(func() {})
	}()
	w.SetPadded(false) // todos los cmd/<app> montan el cromo hasta el borde
	w.SetContent(barra.Montar(contenido))
	recorrido.ComprobarRotacion(t, w, nombre, ui.TactilDePrueba, capturar, preparacion...)
}
