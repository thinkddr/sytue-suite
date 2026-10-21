// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package ficheros

import (
	"context"
	"fmt"
	"os/exec"
)

// copiarImagen lee el PNG como `«class PNGf»` y lo deja en el portapapeles.
func copiarImagen(ruta string) error {
	guion := fmt.Sprintf("set the clipboard to (read (POSIX file %q) as «class PNGf»)", ruta)
	return exec.CommandContext(context.Background(), "osascript", "-e", guion).Run() //nolint:gosec // G204: la ruta es un temporal nuestro
}
