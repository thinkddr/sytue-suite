// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || darwin

package auth

/*
#cgo LDFLAGS: -framework CoreFoundation -framework Security
#include <stdlib.h>
int sytue_llavero_guardar(const char *cuenta, const char *grupo, const void *datos, int n);
int sytue_llavero_borrar(const char *cuenta, const char *grupo);
int sytue_llavero_leer(const char *cuenta, const char *grupo, void **datos, int *n);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func leerDatosApple(cuenta, grupo string) ([]byte, error) {
	c, g := C.CString(cuenta), C.CString(grupo)
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(g))
	var datos unsafe.Pointer
	var n C.int
	if st := C.sytue_llavero_leer(c, g, &datos, &n); st != 0 {
		return nil, fmt.Errorf("auth: el Keychain no se deja leer (OSStatus %d)", int(st))
	}
	if datos == nil {
		return nil, nil
	}
	defer C.free(datos)
	return C.GoBytes(datos, n), nil
}

func guardarDatosApple(cuenta, grupo string, datos []byte) error {
	c, g := C.CString(cuenta), C.CString(grupo)
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(g))
	var inicio unsafe.Pointer
	if len(datos) > 0 {
		inicio = unsafe.Pointer(&datos[0])
	}
	if st := C.sytue_llavero_guardar(c, g, inicio, C.int(len(datos))); st != 0 {
		return fmt.Errorf("auth: el Keychain no deja guardar (OSStatus %d)", int(st))
	}
	return nil
}

func borrarDatosApple(cuenta, grupo string) error {
	c, g := C.CString(cuenta), C.CString(grupo)
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(g))
	if st := C.sytue_llavero_borrar(c, g); st != 0 {
		return fmt.Errorf("auth: el Keychain no deja borrar (OSStatus %d)", int(st))
	}
	return nil
}
