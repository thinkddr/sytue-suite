// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sesion

import "git.sytue.eu/sytue/one/apps/fyne/anfitrion"

// La máquina de Java para el núcleo (`sistema.EnAndroid`) la pone `anfitrion` al cargarse,
// también en las apps que no enlazan `sesion` (Sytue Office); aquí solo queda dar las
// carpetas de la app antes de leer la sesión.
func prepararCasa() { anfitrion.PrepararCasa() }
