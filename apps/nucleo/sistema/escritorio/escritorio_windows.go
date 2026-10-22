// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package escritorio

import (
	"cmp"
	"context"
	"os/exec"
	"syscall"
)

// createNoWindow es CREATE_NO_WINDOW: sin él cada aviso abre y cierra una consola.
const createNoWindow = 0x08000000

// avisar enseña el toast y vuelve sin esperar a PowerShell, que tarda en arrancar.
//
// El toast solo sale si Windows conoce la AUMID —el identificador de paquete—: lo
// declara el instalador en `Software\Classes\AppUserModelId` (`empaquetar.sh`).
func avisar(a Aviso) error {
	if a.Esquema == "" {
		return ErrNoDisponible // sin esquema el clic no tendría por dónde volver
	}
	guion := guionToast(cmp.Or(a.ID, a.Nombre), EnlaceDeAviso(a.Esquema, apuntar(a.AlPulsar), a.Destino), a.Titulo, a.Cuerpo)
	// El guion va entero en base64 y lo arma guionToast con todo escapado: lo que viene
	// del Hub no llega a PowerShell como órdenes.
	cmd := exec.CommandContext(context.Background(), "powershell.exe", //nolint:gosec // ver arriba
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", codificado(guion))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func insignia(string, int) error { return nil }

func hayBandeja() bool { return true }
