// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package ficheros

import "os/exec"

// elegirAplicacion es el selector del escritorio abierto en `/usr/bin`, donde están los
// programas instalados por el sistema (AutoFirma deja ahí `autofirma`).
func elegirAplicacion(titulo string) (string, error) {
	var rutas []string
	var err error
	switch {
	case hay("zenity"):
		rutas, err = elegirCon("zenity", "--file-selection", "--title="+titulo, "--filename=/usr/bin/")
	case hay("kdialog"):
		rutas, err = elegirCon("kdialog", "--title", titulo, "--getopenfilename", "/usr/bin")
	default:
		return "", ErrNoDisponible
	}
	if err != nil || len(rutas) == 0 {
		return "", err
	}
	return rutas[0], nil
}

func hay(orden string) bool { _, err := exec.LookPath(orden); return err == nil }

// abrirCon ejecuta el programa con el fichero como argumento.
func abrirCon(app, fichero string) error { return lanzar(app, fichero) }
