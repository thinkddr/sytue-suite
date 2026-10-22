// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios && !wasm && !test_web_driver

package soltar

import (
	"fyne.io/fyne/v2/driver"

	"github.com/go-gl/glfw/v3.4/glfw"
)

func mismoContexto(c any, gw *glfw.Window) bool {
	m, ok := c.(driver.MacWindowContext)
	return ok && m.NSWindow != 0 && m.NSWindow == uintptr(gw.GetCocoaWindow())
}
