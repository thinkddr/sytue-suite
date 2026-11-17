// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(ios && cgo)

package sistema

// Fuera de iOS no hace falta pedir tiempo: Android deja terminar lo que hay en marcha
// (y lo que dura más va en un JobService), y el escritorio no suspende las apps.
func seguirEnFondo(string) func() { return func() {} }
