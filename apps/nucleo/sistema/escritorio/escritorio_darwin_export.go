// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin

package escritorio

// El `//export` va aparte por lo mismo que el del atajo: cgo no admite definiciones de C
// en el preámbulo de un fichero que exporta.

import "C"

import "net/url"

//export sytueAvisoPulsado
func sytueAvisoPulsado(enlace *C.char) {
	if u, err := url.Parse(C.GoString(enlace)); err == nil {
		Pulsado(u)
	}
}
