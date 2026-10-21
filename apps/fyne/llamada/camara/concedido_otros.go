// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package camara

// Concedido es true fuera de Android: en macOS e iOS Abrir ya ESPERA la respuesta al
// diálogo del sistema (camara_darwin.m), y en los demás no hay permiso que pedir.
func Concedido() bool { return true }
