// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package ficheros

// ordenGuardarComo es el SaveFileDialog por PowerShell, en `-STA` como los demás.
func ordenGuardarComo(titulo, nombre string, extensiones []string) (string, []string, error) {
	return "powershell", []string{"-NoProfile", "-STA", "-Command", guionGuardarWindows(titulo, nombre, extensiones)}, nil
}
