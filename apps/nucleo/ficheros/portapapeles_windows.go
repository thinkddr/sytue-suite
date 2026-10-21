// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package ficheros

import (
	"context"
	"os/exec"
	"strings"
)

// copiarImagen es `Clipboard.SetImage` de Windows Forms, que pide `-STA`.
func copiarImagen(ruta string) error {
	guion := "Add-Type -AssemblyName System.Windows.Forms, System.Drawing;" +
		"[System.Windows.Forms.Clipboard]::SetImage([System.Drawing.Image]::FromFile('" + strings.ReplaceAll(ruta, "'", "''") + "'))"
	return exec.CommandContext(context.Background(), "powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", guion).Run() //nolint:gosec // G204: la ruta es un temporal nuestro
}
