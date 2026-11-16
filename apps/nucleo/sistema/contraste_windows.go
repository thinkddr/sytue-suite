// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package sistema

import "unsafe"

// spiGetHighContrast es `SPI_GETHIGHCONTRAST` de WinUser.h, y hcfHighContrastOn, el bit
// de `HIGHCONTRASTW.dwFlags` que dice que el modo de contraste alto está puesto.
const (
	spiGetHighContrast = 0x0042
	hcfHighContrastOn  = 0x00000001
)

// highContrastW es la estructura `HIGHCONTRASTW` de WinUser.h.
type highContrastW struct {
	cbSize            uint32
	dwFlags           uint32
	lpszDefaultScheme *uint16
}

// masContraste pregunta a user32 sin cgo, con el mismo procedimiento que menosMovimiento.
// Si la llamada falla, no se pide nada: es lo que hace el navegador.
func masContraste() bool {
	if err := systemParametersInfo.Find(); err != nil {
		return false
	}
	hc := highContrastW{cbSize: uint32(unsafe.Sizeof(highContrastW{}))}
	ok, _, _ := systemParametersInfo.Call(spiGetHighContrast, uintptr(hc.cbSize), uintptr(unsafe.Pointer(&hc)), 0)
	return ok != 0 && hc.dwFlags&hcfHighContrastOn != 0
}
