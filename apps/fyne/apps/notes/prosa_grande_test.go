// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"strings"
	"testing"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// The native Mail proof first timed out opening a real 84 KB archived signature:
// its initial wide line repeatedly reshaped every growing word prefix.
func TestLargeInitialLinePreservesTextStyleBoundariesAndCursorOffsets(t *testing.T) {
	text := strings.Repeat("Original private signature ", 3200)
	document := Leer("<p>" + text + "<b>Final</b> tail</p>")
	layout := Maquetar(document, 1e6, tema.Claro)
	if len(layout.Lineas) != 1 || len(layout.Lineas[0].Trozos) != 3 {
		t.Fatal("large initial line lost its style boundaries")
	}
	line := layout.Lineas[0]
	count := utf8.RuneCountInString(text)
	for i, want := range []struct {
		text     string
		from, to int
	}{{text, 0, count}, {"Final", count, count + 5}, {" tail", count + 5, count + 10}} {
		piece := line.Trozos[i]
		if piece.Texto != want.text || piece.Desde != want.from || piece.Hasta != want.to {
			t.Fatal("large initial line changed original text or cursor offsets", i)
		}
		if i > 0 && piece.X != line.Trozos[i-1].X+line.Trozos[i-1].Ancho {
			t.Fatal("style run did not start at the preceding measured boundary", i)
		}
	}
	if line.Hasta != count+10 || textoDeLineas(layout, 0) != text+"Final tail" {
		t.Fatal("large initial line dropped or duplicated source text")
	}
}
