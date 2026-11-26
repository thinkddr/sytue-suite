// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package discocifrado

import (
	"context"
	"strconv"
	"strings"
)

// # Windows
//
// El estado de BitLocker de un volumen se lee SIN elevar por la propiedad del Explorador
// `System.Volume.BitLockerProtection` (la que pinta el candado en «Este equipo»), con el
// `Shell.Application` de PowerShell. `manage-bde -status`, la clase WMI
// `Win32_EncryptableVolume` y `Get-BitLockerVolume` piden administrador, así que no se
// usan. Los valores:
//
//	1 activado · 2 desactivado · 3 cifrándose · 4 descifrándose · 5 suspendido
//	6 activado y bloqueado · 8 esperando a activarse («Cifrado de dispositivo» sin cuenta)
//
// Suspendido y esperando a activarse dejan la clave en claro en el disco: quien tenga el
// equipo lee los datos, así que cuentan como sin cifrar. Cualquier otra cosa (vacío: no
// hay BitLocker en esta edición de Windows o no se deja leer) es «no se sabe». Una ruta
// de red (`\\servidor\…`) no tiene unidad: no se sabe.

// unidad es la letra de unidad de ruta («C:»), o "" si no tiene (una ruta UNC).
func unidad(ruta string) string {
	if len(ruta) >= 2 && ruta[1] == ':' && (ruta[0] >= 'A' && ruta[0] <= 'Z' || ruta[0] >= 'a' && ruta[0] <= 'z') {
		return strings.ToUpper(ruta[:2])
	}
	return ""
}

// guionBitLocker es lo que se le pide a PowerShell para la unidad u («C:»).
func guionBitLocker(u string) string {
	return "(New-Object -ComObject Shell.Application).NameSpace('" + u + `\').Self.ExtendedProperty('System.Volume.BitLockerProtection')`
}

func deWindows(ctx context.Context, ruta string, o orden) Resultado {
	u := unidad(ruta)
	if u == "" {
		return noSeSabe
	}
	salida, err := o(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", guionBitLocker(u))
	if err != nil {
		return noSeSabe
	}
	return leerBitLocker(salida)
}

// leerBitLocker lee el valor de `System.Volume.BitLockerProtection`.
func leerBitLocker(salida string) Resultado {
	n, err := strconv.Atoi(strings.TrimSpace(salida))
	if err != nil {
		return noSeSabe
	}
	switch n {
	case 1, 3, 6:
		return cifradoCon("BitLocker")
	case 2, 4, 5, 8:
		return sinCifrar
	}
	return noSeSabe
}
