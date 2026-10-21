// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !linux

package documentos

// Fuera de Android y Linux no hay dónde enseñar Drive con esta credencial: macOS y iOS
// tienen su File Provider, y Windows el botón de montar de «Conectar tus dispositivos».
var (
	hayProveedor = false
	conectar     = func(Credencial) error { return ErrNoDisponible }
	desconectar  = func(Credencial) {}
)
