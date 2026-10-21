// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	_, w := appConVentana(t, &servicioFalso{})
	recorrido.Comprobar(t, w, 400)
}
