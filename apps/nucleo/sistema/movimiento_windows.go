// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package sistema

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// spiGetClientAreaAnimation es `SPI_GETCLIENTAREAANIMATION` de WinUser.h: el interruptor
// «Mostrar animaciones en Windows» de Accesibilidad → Efectos visuales, que es lo que
// Edge y Chrome traducen a `prefers-reduced-motion: reduce`.
const spiGetClientAreaAnimation = 0x1042

var systemParametersInfo = windows.NewLazySystemDLL("user32.dll").NewProc("SystemParametersInfoW")

// menosMovimiento pregunta a user32 sin cgo. Si la llamada falla (un Windows sin el
// parámetro, anterior a 7), no se pide nada: es lo que hace el navegador.
func menosMovimiento() bool {
	var animar int32 // BOOL
	if err := systemParametersInfo.Find(); err != nil {
		return false
	}
	ok, _, _ := systemParametersInfo.Call(spiGetClientAreaAnimation, 0, uintptr(unsafe.Pointer(&animar)), 0)
	return ok != 0 && animar == 0
}
