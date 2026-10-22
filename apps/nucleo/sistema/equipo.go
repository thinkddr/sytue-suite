// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"os"
	"runtime"
	"strings"
)

// EtiquetaEquipo es con qué nombre sale en Mi cuenta una contraseña de aplicación que
// crea este equipo (el perfil de Apple del Hub, el proveedor de archivos de Drive): el
// del equipo, para saber cuál revocar si se pierde. En iOS el nombre no lo da el sistema.
func EtiquetaEquipo() string {
	if runtime.GOOS == "ios" {
		return "iPhone"
	}
	return EtiquetaCliente("Mac")
}

// EtiquetaCliente es «<cliente> · <equipo>» («Thunderbird · portatil-ada»): la
// contraseña que el Hub crea para un programa de ESTE equipo. Sin nombre de equipo —o
// con el `localhost` que da Android— queda solo el cliente.
func EtiquetaCliente(cliente string) string {
	nombre, err := os.Hostname()
	nombre = strings.TrimSuffix(strings.TrimSpace(nombre), ".local")
	if err != nil || nombre == "" || nombre == "localhost" {
		return cliente
	}
	return cliente + " · " + nombre
}
