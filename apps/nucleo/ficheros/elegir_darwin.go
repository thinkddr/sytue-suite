// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package ficheros

import (
	"fmt"
	"strings"
)

// ordenElegir es `choose file` de AppleScript: el panel de apertura NATIVO del Finder,
// con varios a la vez y filtrado por extensión. Devuelve una ruta POSIX por línea.
func ordenElegir(titulo string, extensiones []string) (string, []string, error) {
	tipos := make([]string, len(extensiones))
	for i, e := range extensiones {
		tipos[i] = fmt.Sprintf("%q", e)
	}
	script := strings.Join([]string{
		fmt.Sprintf("set l to choose file with prompt %q of type {%s} with multiple selections allowed",
			titulo, strings.Join(tipos, ", ")),
		`set out to ""`,
		"repeat with f in l",
		"set out to out & POSIX path of f & linefeed",
		"end repeat",
		"return out",
	}, "\n")
	return "osascript", []string{"-e", script}, nil
}
