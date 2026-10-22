// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package ficheros

import (
	"context"
	"os"
	"os/exec"
)

// copiarImagen usa `wl-copy` en Wayland y `xclip` en X11. Los dos se quedan en segundo
// plano sirviendo la imagen hasta que otro copie algo, así que no se esperan.
func copiarImagen(ruta string) error {
	f, err := os.Open(ruta) //nolint:gosec // G304: un temporal nuestro
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var orden *exec.Cmd
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "" && hay("wl-copy"):
		orden = exec.CommandContext(context.Background(), "wl-copy", "--type", "image/png")
	case hay("xclip"):
		orden = exec.CommandContext(context.Background(), "xclip", "-selection", "clipboard", "-t", "image/png", "-i")
	default:
		return ErrNoDisponible
	}
	orden.Stdin = f
	return orden.Run()
}
