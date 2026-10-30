// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package ficheros

// ordenElegir es el OpenFileDialog de Windows Forms por PowerShell: el cuadro «Abrir»
// del sistema, con varios a la vez. `-STA` porque el diálogo lo exige.
func ordenElegir(titulo string, extensiones []string) (string, []string, error) {
	return "powershell", []string{"-NoProfile", "-STA", "-Command", guionElegirWindows(titulo, extensiones)}, nil
}
