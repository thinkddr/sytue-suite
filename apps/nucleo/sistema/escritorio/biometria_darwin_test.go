// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package escritorio

import "testing"

// TestElMacDiceComoVerificaAlDueno pregunta de verdad a LocalAuthentication —sin enseñar
// ningún diálogo— y comprueba que lo que contesta es una de las formas que la suite sabe
// nombrar. En un Mac de pruebas sin sesión gráfica puede no saber verificar nada.
func TestElMacDiceComoVerificaAlDueno(t *testing.T) {
	nombre, soloContrasena := Biometria()
	switch nombre {
	case "Touch ID", "Face ID", "Optic ID":
	case "":
		if !soloContrasena {
			t.Skip("este Mac no sabe verificar al dueño (¿sin sesión gráfica?)")
		}
	default:
		t.Fatalf("Biometria() = %q", nombre)
	}
	t.Logf("este Mac verifica con %q (solo contraseña: %v)", nombre, soloContrasena)
}
