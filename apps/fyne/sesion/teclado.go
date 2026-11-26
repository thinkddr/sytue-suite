// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import "git.sytue.eu/sytue/one/apps/fyne/anfitrion"

// AltoTeclado son los píxeles (Android) o puntos (iOS) que tapa ahora el teclado en
// pantalla desde abajo, o 0. Vive en `anfitrion`, que no es sesión (C-04).
func AltoTeclado() float32 { return anfitrion.AltoTeclado() }
