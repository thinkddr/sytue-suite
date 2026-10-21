// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package nube

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

// # La carpeta, también para el Explorador
//
// `CfRegisterSyncRoot` basta para que los marcadores funcionen, pero el Explorador no
// sabe de quién es la carpeta: sin columna de estado (la nube y la marca verde), sin
// «Mantener siempre en este dispositivo» ni «Liberar espacio» en el menú, y fuera del
// panel de navegación (medido en Windows Server 2022). Eso lo da registrarla con
// `StorageProviderSyncRootManager`, que es WinRT; se hace con PowerShell, como Windows
// Hello (`escritorio`), para no meter la maquinaria de WinRT en Go. La identidad de la
// carpeta (`raiz`) va en su `Context`; la de su fichero raíz, que este registro no
// guarda, la pone `alPoblar` cuando llega vacía.
//
// Si PowerShell no está o falla, se registra como antes, sin el Explorador.

// idShell es el identificador del registro de `dir`: proveedor, usuario y cuenta,
// separados por «!», que es la forma que exige Windows. La cuenta es una por carpeta
// (FYS-04); la de antes de atarlas sigue con la suya, «Drive», con la que está registrada.
func idShell(dir string) string {
	sid := "local"
	if t, err := syscall.OpenCurrentProcessToken(); err == nil {
		if u, err := t.GetTokenUser(); err == nil {
			if s, err := u.User.Sid.String(); err == nil {
				sid = s
			}
		}
		_ = t.Close()
	}
	cuenta := "Drive"
	if filepath.Base(dir) != carpetaVieja {
		suma := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dir))))
		cuenta = hex.EncodeToString(suma[:8])
	}
	return "SytueDrive!" + sid + "!" + cuenta
}

// guionShell registra (o da de baja, con -Baja) la carpeta en el Explorador. Sale 0 si
// lo ha hecho. Antes de registrar quita el registro sin Explorador de versiones
// anteriores: los dos a la vez no se admiten.
const guionShell = `param([string]$Dir, [string]$Id, [string]$Nombre, [string]$Icono, [switch]$Baja)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$null = [Windows.Storage.StorageFolder, Windows.Storage, ContentType = WindowsRuntime]
$m = [Windows.Storage.Provider.StorageProviderSyncRootManager, Windows.Storage.Provider, ContentType = WindowsRuntime]
if ($Baja) { $m::Unregister($Id); exit 0 }
if ($m::GetCurrentSyncRoots() | Where-Object { $_.Id -eq $Id }) { exit 0 }
Add-Type -Namespace SytueNube -Name Cf -MemberDefinition '[DllImport("cldapi.dll", CharSet=CharSet.Unicode)] public static extern int CfUnregisterSyncRoot(string p);'
$null = [SytueNube.Cf]::CfUnregisterSyncRoot($Dir)
$tarea = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
  $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' })[0]
$t = $tarea.MakeGenericMethod([Windows.Storage.StorageFolder]).Invoke($null, @([Windows.Storage.StorageFolder]::GetFolderFromPathAsync($Dir)))
$t.Wait() | Out-Null
$i = New-Object Windows.Storage.Provider.StorageProviderSyncRootInfo
$i.Id = $Id
$i.Path = $t.Result
$i.DisplayNameResource = $Nombre
$i.IconResource = $Icono
$i.HydrationPolicy = 'Full'
$i.HydrationPolicyModifier = 'AutoDehydrationAllowed'
$i.PopulationPolicy = 'Full'
$i.InSyncPolicy = 'Default'
$i.HardlinkPolicy = 'None'
$i.Version = '1'
$i.AllowPinning = $true
$i.Context = [System.Runtime.InteropServices.WindowsRuntime.WindowsRuntimeBufferExtensions]::AsBuffer([Text.Encoding]::UTF8.GetBytes('raiz'))
$m::Register($i)
`

// enShell corre el guion con PowerShell. Los argumentos van por -File y no dentro del
// texto, así que una ruta con comillas no se interpreta.
func enShell(ctx context.Context, dir string, baja bool) error {
	f, err := os.CreateTemp("", "sytue-nube-*.ps1")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	// UTF-16 con BOM: PowerShell 5.1 lee un .ps1 sin BOM como ANSI.
	u := utf16.Encode([]rune(guionShell))
	b := []byte{0xFF, 0xFE}
	for _, c := range u {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	icono := ""
	if exe, err := os.Executable(); err == nil {
		icono = exe + ",0"
	}
	args := []string{
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", f.Name(),
		"-Dir", dir, "-Id", idShell(dir), "-Nombre", filepath.Base(dir), "-Icono", icono,
	}
	if baja {
		args = append(args, "-Baja")
	}
	ctx, cancelar := context.WithTimeout(ctx, time.Minute)
	defer cancelar()
	cmd := exec.CommandContext(ctx, "powershell.exe", args...)                          //nolint:gosec // guion fijo; lo variable va como argumentos
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd.Run()
}
