// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !ios

package sesion

// AltoTeclado es 0 fuera de Android e iOS: en escritorio no hay teclado en pantalla.
func AltoTeclado() float32 { return 0 }
