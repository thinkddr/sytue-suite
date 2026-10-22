// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows

package escritorio

import (
	"context"
	"os/exec"
	"runtime"
	"time"
)

// lanzarPrograma es `open -a` en macOS, que sale con error si no hay aplicación con ese
// nombre, y el ejecutable del PATH en Linux. En los demás no hay programas que abrir.
func lanzarPrograma(nombre string) error {
	switch runtime.GOOS {
	case "darwin":
		ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelar()
		return exec.CommandContext(ctx, "open", "-a", nombre).Run()
	case "linux":
		ruta, err := exec.LookPath(nombre)
		if err != nil {
			return err
		}
		cmd := exec.CommandContext(context.Background(), ruta)
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}
	return ErrNoDisponible
}
