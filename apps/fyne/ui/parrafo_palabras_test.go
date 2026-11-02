// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestElParrafoOptativoPartePalabrasSinRomperGrafemas(t *testing.T) {
	for _, grafema := range []string{"a\u0301", "👩‍👩‍👧‍👦", "🇪🇸"} {
		texto := strings.Repeat(grafema, 5)
		p := NuevoParrafo(texto, tema.TextoBase, Normal, "text", tema.Claro)
		ancho := Medir(strings.Repeat(grafema, 2), p.Escalon, p.Peso).Width + 0.1
		if len(LineasDe(p, ancho)) != 1 {
			t.Fatal("el reparto normal ha empezado a cortar palabras")
		}
		p.PartirPalabras = true
		lineas := LineasDe(p, ancho)
		if len(lineas) < 2 || strings.Join(lineas, "") != texto {
			t.Fatalf("se pierde texto al repartir %q: %q", texto, lineas)
		}
		for _, linea := range lineas {
			if strings.ReplaceAll(linea, grafema, "") != "" {
				t.Fatalf("se ha separado un grafema de %q: %q", texto, linea)
			}
			if w := Medir(linea, p.Escalon, p.Peso).Width; w > ancho {
				t.Fatalf("la línea %q ocupa %g y solo caben %g", linea, w, ancho)
			}
		}
	}
}
