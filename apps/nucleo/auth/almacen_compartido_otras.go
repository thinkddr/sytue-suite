// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package auth

func cargarCompartida(*AlmacenSesion) (Tokens, bool, error)  { return Tokens{}, false, nil }
func guardarCompartida(*AlmacenSesion, Tokens) (bool, error) { return false, nil }
func migrarCompartida(*AlmacenSesion, bool) (bool, error)    { return false, nil }
