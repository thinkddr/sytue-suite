// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build (!linux && !darwin && !windows) || android || ios || wasm || test_web_driver

package soltar

import "fyne.io/fyne/v2"

// ponytail: en el móvil no se suelta nada sobre una app a pantalla completa, salvo con
// la pantalla partida de Android (`View.OnDragListener` con `ClipData`) o el iPad
// (`UIDropInteraction`). Se releva cuando alguien lo pida y haya dónde probarlo: el
// puente iría en el fork de Fyne, junto al de las URL entrantes.
func alSoltar(fyne.Window, func(Soltado)) {}

func alArrastrar(fyne.Window, func(bool)) {}
