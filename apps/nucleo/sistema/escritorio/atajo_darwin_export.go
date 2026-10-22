// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package escritorio

// El `//export` va en un fichero aparte porque cgo no admite definiciones de C en el
// preámbulo de un fichero que exporta.

import "C"

//export sytueAtajoPulsado
func sytueAtajoPulsado() { pulsarAtajo() }
