// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package ficheros

import (
	"context"
	"os/exec"
)

// copiarImagen es `Clipboard.SetImage` de Windows Forms, que pide `-STA`.
func copiarImagen(ruta string) error {
	return exec.CommandContext(context.Background(), "powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", guionCopiarImagenWindows(ruta)).Run() //nolint:gosec // G204: la ruta va en Base64 dentro de un guion fijo
}
