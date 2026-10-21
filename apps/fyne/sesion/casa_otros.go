// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package sesion

// prepararCasa solo hace falta en Android (java_android.go): en los demás sistemas $HOME
// ya existe.
func prepararCasa() {}
