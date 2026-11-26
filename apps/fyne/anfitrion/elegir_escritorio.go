// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !ios

package anfitrion

import "fyne.io/fyne/v2"

// PonerSelector no hace nada en escritorio: el selector es el del sistema
// (`ficheros.Elegir`), sin toolkit.
func PonerSelector(fyne.App) {}
