// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && cgo

package ficheros

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

// El contenedor compartido del App Group, o NULL si la app no tiene ese grupo en sus
// derechos (firmada sin él, o un perfil que no lo autoriza).
static char *sytue_grupo_ios(const char *grupo) {
	@autoreleasepool {
		NSURL *u = [[NSFileManager defaultManager]
			containerURLForSecurityApplicationGroupIdentifier:[NSString stringWithUTF8String:grupo]];
		if (!u) return NULL;
		return strdup(u.path.fileSystemRepresentation);
	}
}
*/
import "C"

import "unsafe"

// CarpetaDeGrupo es el contenedor compartido del App Group `grupo` (p. ej.
// `group.eu.sytue.office`): una carpeta que leen y escriben todas las apps firmadas con
// ese grupo en sus derechos, para lo que es de toda la edición y no de cada app (el
// perfil, la IA, las cuentas). Sin el derecho, [ErrNoDisponible].
func CarpetaDeGrupo(grupo string) (string, error) {
	cg := C.CString(grupo)
	defer C.free(unsafe.Pointer(cg))
	r := C.sytue_grupo_ios(cg)
	if r == nil {
		return "", ErrNoDisponible
	}
	defer C.free(unsafe.Pointer(r))
	return C.GoString(r), nil
}
