// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package escritorio

import (
	"context"
	"os/exec"
	"syscall"
)

// ejecutarToast corre el guion (ya escapado entero por guionToastProgramado) sin ventana
// y sin esperar a PowerShell.
func ejecutarToast(guion string) error {
	cmd := exec.CommandContext(context.Background(), "powershell.exe", //nolint:gosec // el guion va en base64 y escapado
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", codificado(guion))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
