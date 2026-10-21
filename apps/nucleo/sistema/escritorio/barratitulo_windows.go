// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package escritorio

import (
	"image/color"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procDwmSetWindowAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

// Atributos de DWM (dwmapi.h). Los dos de color son de Windows 11 (22000): en Windows 10
// DWM contesta E_INVALIDARG y el marco se queda con el modo oscuro, que sí existe desde
// la 1809 con el 20.
const (
	dwmwaUseImmersiveDarkMode = 20
	dwmwaCaptionColor         = 35
	dwmwaTextColor            = 36
)

func colorearBarraTitulo(hwnd uintptr, c color.NRGBA, oscuro bool) {
	var modo uint32
	texto := colorRef(color.NRGBA{0, 0, 0, 255})
	if oscuro {
		modo = 1
		texto = colorRef(color.NRGBA{255, 255, 255, 255})
	}
	fondo := colorRef(c)
	dwmPoner(hwnd, dwmwaUseImmersiveDarkMode, unsafe.Pointer(&modo))
	dwmPoner(hwnd, dwmwaCaptionColor, unsafe.Pointer(&fondo))
	dwmPoner(hwnd, dwmwaTextColor, unsafe.Pointer(&texto))
}

// colorRef es el COLORREF de GDI: 0x00BBGGRR.
func colorRef(c color.NRGBA) uint32 { return uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16 }

// dwmPoner fija un atributo de 4 bytes. El error se ignora a propósito: en un Windows
// que no conoce el atributo, lo que queda es la barra del sistema, que es lo de antes.
func dwmPoner(hwnd, atributo uintptr, v unsafe.Pointer) {
	_, _, _ = procDwmSetWindowAttribute.Call(hwnd, atributo, uintptr(v), 4)
}

func identificarVentana(uintptr, string) {}
