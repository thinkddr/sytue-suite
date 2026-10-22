// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows

package escritorio

func graficos() error { return nil }

var ponerDirectorioDLL = func(string) error { return nil }

// AvisarSinGraficos solo existe en Windows, que es donde falta el OpenGL.
func AvisarSinGraficos(string, string) {}

// cederPrimerPlano solo hace falta en Windows (ver CederPrimerPlano).
func cederPrimerPlano() {}
