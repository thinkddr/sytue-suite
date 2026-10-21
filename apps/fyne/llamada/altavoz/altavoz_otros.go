// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js && !darwin && !windows

package altavoz

import "github.com/gen2brain/malgo"

func abrirSistema(capturado, reproducir func([]int16)) (func(), error) {
	return abrirMiniaudio(capturado, reproducir, Sistema{}, malgo.Duplex)
}
