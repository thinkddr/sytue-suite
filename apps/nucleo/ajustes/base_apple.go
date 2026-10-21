// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || darwin

package ajustes

/*
#cgo LDFLAGS: -framework Foundation -framework Security
#include <stdlib.h>
char *sytue_contenedor_suite(const char *grupo, int *autorizado);
*/
import "C"

import (
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

// enGrupoApple dice si la última base resuelta es el App Group: solo ahí se aplica la
// protección completa de iOS (proteccion_ios.go), que es la que exigen los widgets.
var enGrupoApple atomic.Bool

// base es el App Group de la suite si la firma lo trae y el sistema da su carpeta. Si
// no —el interruptor SYTUE_WIDGETS_APPLE apagado, un binario de desarrollo o un perfil
// sin el grupo—, la carpeta de antes de los widgets (baseSinGrupoApple), sin error: una
// app que no arranca por un derecho de Developer Portal es peor que unos widgets vacíos.
func base() (string, error) {
	grupo := C.CString(GrupoContenedorSuite)
	defer C.free(unsafe.Pointer(grupo))
	var autorizado C.int
	ruta := C.sytue_contenedor_suite(grupo, &autorizado)
	if ruta == nil {
		if autorizado != 0 {
			avisarSinContenedor.Do(func() {
				slog.Warn("ajustes: la firma declara el App Group pero el sistema no da su carpeta; se usa la anterior",
					"grupo", GrupoContenedorSuite)
			})
		}
		enGrupoApple.Store(false)
		return baseSinGrupoApple()
	}
	defer C.free(unsafe.Pointer(ruta))
	base := C.GoString(ruta)
	var anteriores []string
	if antigua, err := os.UserConfigDir(); err == nil {
		anteriores = append(anteriores, antigua)
	}
	if runtime.GOOS == "darwin" {
		if u, err := user.Current(); err == nil {
			anteriores = append(anteriores, filepath.Join(u.HomeDir, "Library", "Group Containers", GrupoSuite))
		}
	}
	if err := migrarPreferencias(base, anteriores...); err != nil {
		return "", err
	}
	enGrupoApple.Store(true)
	return base, nil
}

var avisarSinContenedor sync.Once
