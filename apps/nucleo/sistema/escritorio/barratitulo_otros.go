// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !((darwin && !ios) || windows || (linux && !android && cgo))

package escritorio

import "image/color"

// En el móvil no hay barra de título, y un Linux sin cgo no tiene con qué hablar con X11.
func colorearBarraTitulo(uintptr, color.NRGBA, bool) {}

func identificarVentana(uintptr, string) {}
