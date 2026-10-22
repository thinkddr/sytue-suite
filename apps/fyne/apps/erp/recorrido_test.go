// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	p := pantallaDePrueba(t)
	w := test.NewTempWindow(t, p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	if os.Getenv("SYTUE_CAPTURAS_PLAY") != "" {
		_, w = pantallaArnes(t, fyne.NewSize(360, 640)) // la lista de facturas, con datos
	}
	recorrido.Comprobar(t, w, 400)
}
