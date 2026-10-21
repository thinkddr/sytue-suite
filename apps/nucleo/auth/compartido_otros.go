// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !darwin && !ios && !android && !js

package auth

func cuentaCompartida(string) bool            { return false }
func cargarCompartida(string) (Tokens, error) { return Tokens{}, nil }
func guardarCompartida(string, Tokens) error  { return nil }

func leerSecretoCompartido(string) ([]byte, error)  { return nil, nil }
func guardarSecretoCompartido(string, []byte) error { return nil }
func borrarSecretoCompartido(string) error          { return nil }
