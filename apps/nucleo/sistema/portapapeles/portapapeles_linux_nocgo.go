// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android && !cgo

package portapapeles

// Sin cgo no hay Xlib: solo el portapapeles de Wayland, que va en Go.
func leer() ([]byte, error) { return leerWayland() }
