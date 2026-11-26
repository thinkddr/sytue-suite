// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package ficheros

import "fmt"

// ordenGuardarComo es `choose file name` de AppleScript: el panel de guardar del Finder,
// que pregunta él si se reemplaza un fichero que ya existe. AppleScript no filtra por
// tipo al guardar; la extensión la pone [GuardarComo] si falta.
func ordenGuardarComo(titulo, nombre string, _ []string) (string, []string, error) {
	return "osascript", []string{"-e", fmt.Sprintf("return POSIX path of (choose file name with prompt %q default name %q)", titulo, nombre)}, nil
}
