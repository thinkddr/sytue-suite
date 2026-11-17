// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || darwin

// gocritic lee este fichero ya pasado por cgo y da avisos de otro código («dupImport»
// de `C` y `unsafe`, «dupSubExpr» de líneas desplazadas): se silencia en el fichero,
// y el resto de linters sí lo miran.
//
//nolint:gocritic // cgo: los avisos no son de este fichero, ver arriba
package auth

/*
#cgo LDFLAGS: -framework CoreFoundation -framework Security
#include <stdlib.h>
int sytue_llavero_guardar(const char *cuenta, const char *grupo, const void *datos, int n);
int sytue_llavero_borrar(const char *cuenta, const char *grupo);
int sytue_llavero_leer(const char *cuenta, const char *grupo, void **datos, int *n);
char *sytue_llavero_bundle(void);
int sytue_llavero_existe(const char *cuenta, const char *grupo, int *existe);
int sytue_llavero_mover(const char *cuenta, const char *origen, const char *destino);
char *sytue_llavero_esquema(void);
*/
import "C" //nolint:gocritic // CGO exige importar C por separado

import (
	"fmt"
	"sync"
	"unsafe" //nolint:gocritic // dupImport falso positivo con import C

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

func leerDatosApple(cuenta, grupo string) ([]byte, error) {
	if grupo == "" {
		own, err := grupoPrivadoApple()
		if err != nil {
			return nil, err
		}
		grupo = own
	}
	c, g := C.CString(cuenta), C.CString(grupo)
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(g))
	var datos unsafe.Pointer
	var n C.int
	if st := C.sytue_llavero_leer(c, g, &datos, &n); st != 0 { //nolint:gocritic // dupSubExpr falso positivo en código generado por cgo
		return nil, fmt.Errorf("auth: el Keychain no se deja leer (OSStatus %d)", int(st))
	}
	if datos == nil {
		return nil, nil
	}
	defer C.free(datos)
	return C.GoBytes(datos, n), nil
}

func guardarDatosApple(cuenta, grupo string, datos []byte) error {
	if grupo == "" {
		own, err := grupoPrivadoApple()
		if err != nil {
			return err
		}
		grupo = own
	}
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
	if grupo == "" {
		own, err := grupoPrivadoApple()
		if err != nil {
			return err
		}
		grupo = own
	}
	c, g := C.CString(cuenta), C.CString(grupo)
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(g))
	if st := C.sytue_llavero_borrar(c, g); st != 0 {
		return fmt.Errorf("auth: el Keychain no deja borrar (OSStatus %d)", int(st))
	}
	return nil
}

var grupoPrivadoApple = sync.OnceValues(func() (string, error) {
	bundle := C.sytue_llavero_bundle()
	if bundle == nil {
		return "", ErrGrupoPrivadoApple
	}
	defer C.free(unsafe.Pointer(bundle))
	return grupoPrivadoDesdeBundle(C.GoString(bundle), ajustes.GrupoSuite)
})

// Metadata only: this never requests kSecReturnData or exposes a legacy value.
func existeDatosApple(cuenta, grupo string) (bool, error) {
	if grupo == "" {
		own, err := grupoPrivadoApple()
		if err != nil {
			return false, err
		}
		grupo = own
	}
	c, g := C.CString(cuenta), C.CString(grupo)
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(g))
	var exists C.int
	if status := C.sytue_llavero_existe(c, g, &exists); status != 0 {
		return false, fmt.Errorf("auth: keychain metadata unavailable (OSStatus %d)", int(status))
	}
	return exists != 0, nil
}

func esquemaPrivadoApple() (string, error) {
	value := C.sytue_llavero_esquema()
	if value == nil {
		return "", ErrGrupoPrivadoApple
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value), nil
}

func moverDatosApple(cuenta, origen, destino string) error {
	c, o, d := C.CString(cuenta), C.CString(origen), C.CString(destino)
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(o))
	defer C.free(unsafe.Pointer(d))
	if status := C.sytue_llavero_mover(c, o, d); status != 0 {
		return fmt.Errorf("auth: scoped keychain migration failed (OSStatus %d)", int(status))
	}
	return nil
}
