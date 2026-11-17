// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build ios || (darwin && tienda)

package auth

import "git.sytue.eu/sytue/one/apps/nucleo/ajustes"

type keychainMigration struct{}

func (keychainMigration) Read(account, group string) ([]byte, error) {
	return leerDatosApple(account, group)
}

func (keychainMigration) Move(account, from, to string) error {
	return moverDatosApple(account, from, to)
}

// MigrarSecretoPrivado is an explicit recovery path for existing private data.
// The signed native scheme and domain validator must both accept the material.
// Ordinary reads never search Suite. Ambiguous destinations remain unchanged.
func MigrarSecretoPrivado(account, scheme string, validate func([]byte) error) ([]byte, error) {
	if !cuentaPrivadaMigrable(account) || validate == nil {
		return nil, ErrSecretoLegadoCompartido
	}
	actual, err := esquemaPrivadoApple()
	if err != nil || actual != scheme {
		return nil, ErrGrupoPrivadoApple
	}
	own, err := grupoPrivadoApple()
	if err != nil {
		return nil, err
	}
	return migrarSecretoPrivado(keychainMigration{}, account, own, ajustes.GrupoSuite, validate)
}

func EsquemaAplicacionPrivada() (string, error) { return esquemaPrivadoApple() }
