// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !ios

package escritorio

// Fuera de Android e iOS no hay nada que preparar: macOS pregunta con el primer aviso,
// y Linux y Windows no preguntan.
func prepararAvisos(string) error { return nil }
