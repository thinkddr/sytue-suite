// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && !cgo

package sesion

// AltoTeclado devuelve 0 cuando se compila sin cgo.
func AltoTeclado() float32 { return 0 }
