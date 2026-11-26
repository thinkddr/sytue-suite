// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaTablaOcupaElAnchoDeLaNota: como la web (`.sytue-prose table`: `w-full` y
// `table-layout: fixed`), una tabla ocupa todo el ancho de la nota y sus columnas se lo
// reparten a partes iguales, lleve lo que lleve. Una sin texto salía como una tira
// estrecha ajustada a sus celdas vacías (lo vio el dueño, 2026-11-26); con texto corto,
// igual de estrecha; y la columna de una palabra larga se comía a las demás.
func TestLaTablaOcupaElAnchoDeLaNota(t *testing.T) {
	const ancho = 600
	for nombre, html := range map[string]string{
		"vacía":         `<table><tbody><tr><td></td><td></td><td></td></tr><tr><td></td><td></td><td></td></tr></tbody></table>`,
		"texto corto":   `<table><tbody><tr><td>a</td><td>b</td><td>c</td></tr></tbody></table>`,
		"desigual":      `<table><tbody><tr><td>una celda con bastante más texto que las otras</td><td>b</td><td>c</td></tr></tbody></table>`,
		"recién puesta": `<table style="border-collapse: collapse; width: 100%"><tbody><tr><td></td><td></td><td></td></tr></tbody></table>`,
	} {
		m := Maquetar(Leer(html), ancho, tema.Claro)
		// Las rayas verticales de la rejilla: las X donde empieza cada una.
		xs := map[float32]bool{}
		var izq, der float32 = ancho, 0
		for _, a := range m.Adornos {
			if a.Tipo != adornoCaja || a.An > a.Al {
				continue
			}
			xs[a.X] = true
			izq, der = min(izq, a.X), max(der, a.X+a.An)
		}
		if len(xs) != 4 {
			t.Errorf("%s: %d rayas verticales y no las 4 de tres columnas: %v", nombre, len(xs), xs)
			continue
		}
		if izq > 0.5 || der < ancho-0.5 {
			t.Errorf("%s: la tabla va de %v a %v y no ocupa los %v de la nota", nombre, izq, der, ancho)
		}
		bordes := make([]float32, 0, len(xs))
		for x := range xs {
			bordes = append(bordes, x)
		}
		slices.Sort(bordes)
		for i := 1; i < len(bordes)-1; i++ {
			if d1, d2 := bordes[i]-bordes[i-1], bordes[i+1]-bordes[i]; d1-d2 > 0.5 || d2-d1 > 0.5 {
				t.Errorf("%s: columnas desiguales: %v", nombre, bordes)
				break
			}
		}
	}
}
