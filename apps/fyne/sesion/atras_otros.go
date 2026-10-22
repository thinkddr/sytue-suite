// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package sesion

// registrarAtras solo hace falta en Android (atras_android.go): en iOS no hay «atrás»
// del sistema y en escritorio es Escape.
func registrarAtras() {}
