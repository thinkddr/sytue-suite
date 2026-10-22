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
// ponytail: no pasa por el portal de XDG (`org.freedesktop.portal.FileChooser`), que es
// lo que haría falta dentro de un Flatpak; ahí no hay zenity y sale ErrNoDisponible. Se
// cierra con la llamada por D-Bus que ya usa Revelar, cuando se empaquete así.
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
