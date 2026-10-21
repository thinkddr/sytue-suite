// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android && !wasm && !test_web_driver

package soltar

import (
	"unsafe"

	"fyne.io/fyne/v2/driver"

	"github.com/go-gl/glfw/v3.4/glfw"
)

// mismoContexto: Fyne da el contexto de X11 o el de Wayland según con cuál arrancó GLFW,
// y solo se pregunta a GLFW por ese, porque el otro devuelve un error de plataforma.
func mismoContexto(c any, gw *glfw.Window) bool {
	switch c := c.(type) {
	case driver.X11WindowContext:
		return c.WindowHandle != 0 && c.WindowHandle == uintptr(gw.GetX11Window())
	case driver.WaylandWindowContext:
		return c.WaylandSurface != 0 && c.WaylandSurface == uintptr(unsafe.Pointer(gw.GetWaylandWindow()))
	}
	return false
}
