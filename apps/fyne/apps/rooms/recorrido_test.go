// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	sa, _, w := salaEn(t, servicioConDos(), 1280)
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	comprobarHistorialRotacion(t, w, h, "antes")
	montaje.ComprobarRotacion(t, w, "rooms/canal", iconos.Rooms, true, func() {
		comprobarHistorialRotacion(t, w, h, "girado")
	})
	recorrido.Comprobar(t, w, 400)
}
