// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !ios && (!darwin || !tienda)

package auth

func MigrarSecretoPrivado(string, string, func([]byte) error) ([]byte, error) {
	return nil, ErrGrupoPrivadoApple
}
func EsquemaAplicacionPrivada() (string, error) { return "", ErrGrupoPrivadoApple }
