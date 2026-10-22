// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && cgo

package ficheros

// El `//export` va aparte porque cgo no admite definiciones de C en el
// preámbulo de un fichero que exporta.

import "C"

import "strings"

//export sytue_elegir_ios_retorno
func sytue_elegir_ios_retorno(crudos *C.char) {
	var rutas []string
	if crudos != nil {
		s := C.GoString(crudos)
		for _, l := range strings.Split(s, "\n") {
			l = strings.TrimSpace(l)
			if l != "" {
				rutas = append(rutas, l)
			}
		}
	}
	elegirMu.Lock()
	ch := elegirCanal
	elegirMu.Unlock()
	if ch != nil {
		ch <- rutas
	}
}
