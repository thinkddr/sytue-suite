// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package escritorio

import (
	"context"
	"encoding/base64"
	"errors"
	"os/exec"
	"syscall"
)

// En Windows se nombra sin preguntar: saber si Hello está configurado cuesta arrancar
// PowerShell, y eso no se hace al pintar un menú. Si no lo está, [Verificar] lo dice.
func biometria() (string, bool) { return "Windows Hello", false }

// guionHello pide la verificación con `UserConsentVerifier` y sale con su resultado:
// 0 es Verified; 10 es que Hello no está disponible en este equipo.
const guionHello = `$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Runtime.WindowsRuntime
[Windows.Security.Credentials.UI.UserConsentVerifier, Windows.Security.Credentials.UI, ContentType = WindowsRuntime] > $null
$tarea = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
  $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' })[0]
function Esperar($op, $tipo) { $t = $tarea.MakeGenericMethod($tipo).Invoke($null, @($op)); $t.Wait() | Out-Null; $t.Result }
$hay = Esperar ([Windows.Security.Credentials.UI.UserConsentVerifier]::CheckAvailabilityAsync()) ([Windows.Security.Credentials.UI.UserConsentVerifierAvailability])
if ([int]$hay -ne 0) { exit 10 }
$motivo = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:SYTUE_MOTIVO))
$r = Esperar ([Windows.Security.Credentials.UI.UserConsentVerifier]::RequestVerificationAsync($motivo)) ([Windows.Security.Credentials.UI.UserConsentVerificationResult])
exit [int]$r
`

// base64Motivo es el motivo para pasarlo por el entorno a PowerShell, que lo decodifica
// como UTF-8: así no hay comillas que escapar.
func base64Motivo(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// verificar pide Windows Hello (huella, cara o PIN). El motivo viaja en base64 por el
// entorno, así que ningún texto llega a PowerShell como órdenes.
//
// ponytail: sin verlo en un Windows. Desde una app de escritorio el diálogo de Hello
// puede salir detrás de la ventana; lo arregla pedirlo con la HWND
// (`IUserConsentVerifierInterop`), que pide COM desde Go.
func verificar(ctx context.Context, motivo string) error {
	cmd := exec.CommandContext(ctx, "powershell.exe", //nolint:gosec // guion fijo; el motivo va por el entorno
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", codificado(guionHello))
	cmd.Env = append(cmd.Environ(), "SYTUE_MOTIVO="+base64Motivo(motivo))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	err := cmd.Run()
	var salida *exec.ExitError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &salida) && salida.ExitCode() == 10:
		return ErrNoDisponible
	case errors.As(err, &salida):
		return ErrNoVerificado
	}
	return ErrNoDisponible // sin PowerShell no hay Hello que pedir
}
