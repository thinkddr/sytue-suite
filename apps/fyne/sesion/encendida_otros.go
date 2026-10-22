// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package sesion

import "fyne.io/fyne/v2"

// PantallaEncendida pide que la pantalla no se apague (o deja que se apague): en iOS y
// en escritorio lo hace el driver de Fyne.
func PantallaEncendida(a fyne.App, si bool) { a.Driver().SetDisableScreenBlanking(si) }
