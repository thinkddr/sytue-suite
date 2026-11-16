// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package ficheros

import (
	"os/exec"
	"strings"
)

// ordenElegir usa el selector del escritorio que haya: zenity (GNOME y casi todos) y, si
// no, kdialog (KDE). Los dos dan una ruta por línea.
//
// Sin ninguno de los dos devuelve ErrNoDisponible y ElegirConTipos pasa al portal de
// XDG (`org.freedesktop.portal.FileChooser`, en sistema_linux.go), que es lo que hay
// dentro de un Flatpak y en los escritorios que no traen ni zenity ni kdialog.
func ordenElegir(titulo string, extensiones []string) (string, []string, error) {
	patrones := make([]string, len(extensiones))
	for i, e := range extensiones {
		patrones[i] = "*." + e
	}
	if ruta, err := exec.LookPath("zenity"); err == nil {
		return ruta, []string{
			"--file-selection", "--multiple", "--separator=\n", "--title=" + titulo,
			"--file-filter=" + titulo + " | " + strings.Join(patrones, " "),
		}, nil
	}
	if ruta, err := exec.LookPath("kdialog"); err == nil {
		return ruta, []string{
			"--title", titulo, "--getopenfilename", ".", strings.Join(patrones, " "),
			"--multiple", "--separate-output",
		}, nil
	}
	return "", nil, ErrNoDisponible
}

// ordenElegirCarpeta es el selector de carpetas de zenity o de kdialog; sin ninguno,
// ErrNoDisponible y [ElegirCarpeta] pasa al portal.
func ordenElegirCarpeta(titulo string) (string, []string, error) {
	if ruta, err := exec.LookPath("zenity"); err == nil {
		return ruta, []string{"--file-selection", "--directory", "--title=" + titulo}, nil
	}
	if ruta, err := exec.LookPath("kdialog"); err == nil {
		return ruta, []string{"--title", titulo, "--getexistingdirectory", "."}, nil
	}
	return "", nil, ErrNoDisponible
}
