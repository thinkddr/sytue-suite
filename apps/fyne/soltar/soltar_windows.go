// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows && !wasm && !test_web_driver

package soltar

import (
	"unsafe"

	"fyne.io/fyne/v2/driver"

	"github.com/go-gl/glfw/v3.4/glfw"
)

func mismoContexto(c any, gw *glfw.Window) bool {
	w, ok := c.(driver.WindowsWindowContext)
	return ok && w.HWND != 0 && w.HWND == uintptr(unsafe.Pointer(gw.GetWin32Window()))
}
