// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	var portada fyne.CanvasObject = lanzadorDePrueba(t)
	if os.Getenv("SYTUE_CAPTURAS_PLAY") != "" {
		portada = container.NewVScroll(portada) // en el teléfono la portada se desplaza
	}
	w := test.NewTempWindow(t, portada)
	w.Resize(fyne.NewSize(1280, 800))
	recorrido.Comprobar(t, w, 400)
}
