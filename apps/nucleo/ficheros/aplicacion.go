// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"errors"
	"os"
	"path/filepath"
)

// ElegirAplicacion pide a la persona que SEÑALE una aplicación del equipo —la de firma
// de su país, por ejemplo— y devuelve su ruta: `choose application` en macOS, el
// selector filtrado a `.exe` en Windows y el selector en `/usr/bin` en Linux (ahí los
// programas no llevan extensión). Cancelar devuelve "", sin error. BLOQUEA: desde una
// rutina propia.
//
// No se adivina el ejecutable a propósito: cada aplicación se instala en un sitio en
// cada sistema y cambia de nombre entre versiones, y una tabla de rutas funciona en la
// máquina de quien la escribió y falla en silencio en la de los demás.
func ElegirAplicacion(titulo string) (string, error) { return elegirAplicacion(titulo) }

// AbrirCon abre `fichero` con la aplicación `app` que devolvió ElegirAplicacion. Solo
// ejecuta lo que la persona señaló con el ratón y solo le pasa un fichero de verdad.
func AbrirCon(app, fichero string) error {
	abs, err := local(fichero)
	if err != nil {
		return err
	}
	if _, err := os.Stat(app); err != nil {
		return errors.New("ficheros: la aplicación " + filepath.Base(app) + " ya no está")
	}
	return abrirCon(app, abs)
}
