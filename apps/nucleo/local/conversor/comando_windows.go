// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package conversor

import (
	"os/exec"
	"syscall"
)

// ocultarVentana: ffmpeg es un programa de consola y Windows le abriría una ventana negra
// encima de la app mientras convierte.
func ocultarVentana(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
