// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"regexp"
	"slices"
	"strings"
)

// mencion es `@` seguido de una dirección, sin nada pegado delante: `@ana@empresa.es`.
var mencion = regexp.MustCompile(`(?:^|[\s(])@([^\s@()<>,;]+@[^\s@()<>,;]+\.[^\s@()<>,;]+)`)

// Mencionados son las direcciones mencionadas en un texto (`@ana@empresa.es`), en
// minúsculas y sin repetir. Es la forma de mencionar a alguien en toda la suite: un
// comentario de Docs, una nota de Notepads.
//
// ponytail: hasta cinco por texto; más ya no es mencionar, es un correo masivo.
func Mencionados(texto string) []string {
	var out []string
	for _, m := range mencion.FindAllStringSubmatch(texto, -1) {
		e := strings.ToLower(strings.TrimRight(m[1], "."))
		if !slices.Contains(out, e) && len(out) < 5 {
			out = append(out, e)
		}
	}
	return out
}
