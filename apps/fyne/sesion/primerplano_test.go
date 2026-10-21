// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// TestLaVidaDeLaAppLlegaATodos: Fyne tiene UN receptor por app para volver y para irse
// al fondo. La barra de la suite y Cast se apuntan los dos: tienen que enterarse los
// dos, cada uno de lo suyo, en el orden en que llegaron.
//
// VISTO EN ROJO con `engancharVida` apuntando directamente al último que se apunta
// (lo que hacía la barra con `SetOnEnteredForeground`): el primero no se entera.
func TestLaVidaDeLaAppLlegaATodos(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	s := Abrir(a, "sytue.eu")
	var orden []string
	s.AlVolver(func() { orden = append(orden, "barra") })
	s.AlVolver(func() { orden = append(orden, "cast") })
	s.AlFondo(func() { orden = append(orden, "pausa") })
	s.AlVolver(nil) // no rompe nada

	lc, ok := a.Lifecycle().(interface {
		OnEnteredForeground() func()
		OnExitedForeground() func()
	})
	if !ok {
		t.Fatal("el ciclo de vida de Fyne ya no enseña sus receptores")
	}
	lc.OnExitedForeground()()
	lc.OnEnteredForeground()()
	if got := len(orden); got != 3 || orden[0] != "pausa" || orden[1] != "barra" || orden[2] != "cast" {
		t.Errorf("al irse y volver se avisó a %v; se esperaba [pausa barra cast]", orden)
	}
}
