// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

/*
#include <stdlib.h>
int sytue_fichero_proteger(const char *ruta);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// protegerFichero aplica NSFileProtectionComplete solo en el App Group, que es lo que leen
// los widgets. Fuera de él (widgets apagados) los ficheros conservan la protección de
// siempre: Complete los haría ilegibles con el teléfono bloqueado.
func protegerFichero(ruta string) error {
	if !enGrupoApple.Load() {
		return nil
	}
	c := C.CString(ruta)
	defer C.free(unsafe.Pointer(c))
	if codigo := C.sytue_fichero_proteger(c); codigo != 0 {
		return fmt.Errorf("ajustes: no se pudo aplicar la protección completa de iOS (código %d)", int(codigo))
	}
	return nil
}
