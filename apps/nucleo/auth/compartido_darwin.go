// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios && !tienda

package auth

/*
#include <stdlib.h>
int sytue_llavero_autorizado(const char *grupo);
*/
import "C" //nolint:gocritic // CGO exige importar C por separado

import (
	"log/slog"
	"sync"
	"unsafe" //nolint:gocritic // dupImport falso positivo con import C

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

var puedeCompartirApple = sync.OnceValue(func() bool {
	grupo := C.CString(ajustes.GrupoSuite)
	defer C.free(unsafe.Pointer(grupo))
	return C.sytue_llavero_autorizado(grupo) != 0
})

func cuentaCompartida(cuenta string) bool {
	return enGrupoSuite(cuenta) && puedeCompartirApple()
}

func cargarCuentaCompartida(cuenta string) (Tokens, error) {
	datos, err := leerDatosApple(cuenta, ajustes.GrupoSuite)
	if err != nil || datos == nil {
		return Tokens{}, err
	}
	return recuperar(datos)
}

func guardarCuentaCompartida(cuenta string, t Tokens) error {
	if t.Refresh == "" {
		return borrarDatosApple(cuenta, ajustes.GrupoSuite)
	}
	datos, err := serializar(t)
	if err != nil {
		return err
	}
	return guardarDatosApple(cuenta, ajustes.GrupoSuite, datos)
}

func leerSecretoCompartido(cuenta string) ([]byte, error) {
	datos, err := leerDatosApple(cuenta, ajustes.GrupoSuite)
	if err != nil || datos != nil {
		return datos, err
	}
	// Conservar el testigo anterior de bloqueo después de actualizar el DMG.
	datos, err = leerSecretoLegado(cuenta)
	if err == nil && len(datos) > 0 {
		errMig := guardarDatosApple(cuenta, ajustes.GrupoSuite, datos)
		if errMig != nil {
			slog.Warn("auth: se conserva el bloqueo anterior; no se pudo migrar al grupo de la suite", "error", errMig)
		}
	}
	return datos, err
}

func guardarSecretoCompartido(cuenta string, datos []byte) error {
	return guardarDatosApple(cuenta, ajustes.GrupoSuite, datos)
}

func borrarSecretoCompartido(cuenta string) error {
	return borrarDatosApple(cuenta, ajustes.GrupoSuite)
}
