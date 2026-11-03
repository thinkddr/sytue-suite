// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"

	"fyne.io/fyne/v2"
)

func TestMantenerRegistroSoloCambiaLaSeleccion(t *testing.T) {
	abiertas, marcadas := 0, 0
	fila := NuevaFilaERP("Registro", func() { abiertas++ })
	tarjeta := NuevaTarjetaReg("Registro", func() { abiertas++ })
	fila.AlMarcar = func() { marcadas++ }
	tarjeta.AlMarcar = func() { marcadas++ }
	for _, o := range []fyne.SecondaryTappable{fila, tarjeta} {
		o.TappedSecondary(nil)
	}
	if abiertas != 0 || marcadas != 2 {
		t.Fatalf("mantener abre %d registros y cambia %d selecciones", abiertas, marcadas)
	}
	fila.Tapped(nil)
	tarjeta.Tapped(nil)
	if abiertas != 2 || marcadas != 2 {
		t.Fatal("la pulsación normal no conserva abrir el registro")
	}
}
