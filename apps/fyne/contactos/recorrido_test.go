// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana"), persona("bruno", "Bruno"), fichaDeMuestra()}}
	if os.Getenv("SYTUE_CAPTURAS_PLAY") != "" {
		s.contactos = pantallaDeMuestra(t).Contactos // la libreta con nombres de verdad
	}
	a := appDePrueba(t, s)
	if os.Getenv("SYTUE_CAPTURAS_PLAY") != "" {
		a.Cargar()
		s.correrCola()
	}
	ws := fyne.CurrentApp().Driver().AllWindows()
	recorrido.Comprobar(t, ws[len(ws)-1], 400)
}
