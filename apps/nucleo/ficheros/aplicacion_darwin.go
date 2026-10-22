// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package ficheros

import (
	"fmt"
	"strings"
)

// elegirAplicacion es `choose application` de AppleScript: la lista de aplicaciones del
// sistema, con el buscador del propio macOS. Devuelve la ruta POSIX del `.app`.
func elegirAplicacion(titulo string) (string, error) {
	rutas, err := elegirCon("osascript", "-e",
		fmt.Sprintf("POSIX path of (choose application with prompt %q as alias)", titulo))
	if err != nil || len(rutas) == 0 {
		return "", err
	}
	return strings.TrimSuffix(rutas[0], "/"), nil
}

// abrirCon es `open -a`: un `.app` es un paquete, y quien sabe entrar en él es `open`.
func abrirCon(app, fichero string) error { return lanzar("open", "-a", app, fichero) }
