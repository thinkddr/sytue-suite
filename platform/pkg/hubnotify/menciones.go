// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"regexp"
	"slices"
	"strings"
)

// blanco es el `\s` de JavaScript, que es con el que la web pinta una mención
// (`conMenciones`, client/hashtags.ts) y con el que Fyne la abre (`blancoJS`,
// apps/fyne/ui/menciones.go, otro módulo). El `\s` de RE2 se queda en el ASCII, y el
// espacio duro que el editor deja tras una mención al final de un párrafo (`&nbsp;`,
// U+00A0 al pasar a texto) quedaba pegado a la dirección: `ana@acme.test\u00a0` no es
// nadie, y no se avisaba.
const blanco = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

// mencion es `@` seguido de una dirección, sin nada pegado delante: `@ana@empresa.es`.
var mencion = regexp.MustCompile(`(?:^|[` + blanco + `(])@([^` + blanco + `@()<>,;]+@[^` + blanco + `@()<>,;]+\.[^` + blanco + `@()<>,;]+)`)

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
