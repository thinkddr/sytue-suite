// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !linux || android

package escritorio

// Fuera de Linux el aviso lleva su vuelta consigo (el enlace, el `userInfo`): no hay
// nada que dejar listo al arrancar.
func recibirAvisos(string) error { return nil }
