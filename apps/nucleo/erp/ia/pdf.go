// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"cmp"
	"math"
	"slices"
	"strings"
)

// FragmentoPDF es un trozo de texto de una página con su posición (en puntos, con
// la y creciendo hacia arriba), como lo da el motor PDF (apps/nucleo/pdf.TextSpans).
type FragmentoPDF struct {
	Texto string  `json:"texto"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

// TextoDePDF pone los fragmentos de cada página en orden de lectura: de arriba abajo
// y, en la misma línea, de izquierda a derecha. Un fragmento a menos de 3 puntos de
// altura del primero de la línea va en esa línea. Las páginas van separadas por una
// línea en blanco. Es el texto que se le pasa a la IA para rellenar una ficha.
func TextoDePDF(paginas [][]FragmentoPDF) string {
	var b strings.Builder
	for _, fragmentos := range paginas {
		slices.SortStableFunc(fragmentos, func(a, c FragmentoPDF) int { return cmp.Compare(c.Y, a.Y) })
		var lineas [][]FragmentoPDF
		for _, f := range fragmentos {
			if n := len(lineas); n > 0 && math.Abs(lineas[n-1][0].Y-f.Y) <= 3 {
				lineas[n-1] = append(lineas[n-1], f)
			} else {
				lineas = append(lineas, []FragmentoPDF{f})
			}
		}
		for i, linea := range lineas {
			slices.SortStableFunc(linea, func(a, c FragmentoPDF) int { return cmp.Compare(a.X, c.X) })
			if i > 0 {
				b.WriteByte('\n')
			}
			for j, f := range linea {
				if j > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(strings.TrimSpace(f.Texto))
			}
		}
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}
