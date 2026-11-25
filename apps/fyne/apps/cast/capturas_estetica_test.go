// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestCapturasEsteticas deja con SYTUE_CAPTURAS el inicio de Cast en las ocho
// combinaciones de la auditoría del 2026-11-24 (ver `montaje.Capturar`): en Carril su
// cabecera era una franja blanca sobre el gris.
func TestCapturasEsteticas(t *testing.T) {
	a, w := appDePrueba(t, servicioCompleto(), true)
	a.Elegir = func() ([]string, error) { return nil, nil }
	a.Arrancar()
	a.Ir(RutaInicio)
	montaje.Capturar(t, w, "cast-inicio", iconos.Cast, func(montaje.MedidaCaptura) {
		a.Repintar()
	})
}
