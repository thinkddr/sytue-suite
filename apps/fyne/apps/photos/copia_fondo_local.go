// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build edicionlocal

package photos

import "sync/atomic"

// appEnFondo existe también en Sytue Office para que identidad.go sea el mismo en las dos
// ediciones, pero aquí no lo lee nadie: sin bóveda (GaleriaCifrada es nil) no se guarda
// nada, y la copia con la app cerrada es de la conectada (copia_fondo.go).
var appEnFondo atomic.Pointer[App]
