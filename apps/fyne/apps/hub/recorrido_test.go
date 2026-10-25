// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	// cmd/hub también monta la portada dentro de un desplazador vertical.
	var portada fyne.CanvasObject = container.NewVScroll(lanzadorDePrueba(t))
	w := test.NewTempWindow(t, portada)
	w.Resize(fyne.NewSize(1280, 800))
	montaje.ComprobarRotacion(t, w, "hub", iconos.One, true)
	recorrido.Comprobar(t, w, 400)
}
