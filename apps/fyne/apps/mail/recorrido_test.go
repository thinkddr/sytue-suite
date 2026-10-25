// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	b, _ := buzonDePrueba(t, servicioDeEjemplo())
	w := test.NewTempWindow(t, b.P.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	b.Arrancar(context.Background())
	montaje.ComprobarRotacion(t, w, "mail/buzon", iconos.Mail, true)
	recorrido.Comprobar(t, w, 400)
}
