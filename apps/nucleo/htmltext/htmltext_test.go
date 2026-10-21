// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package htmltext

import "testing"

func TestText(t *testing.T) {
	for html, quiero := range map[string]string{
		"sin formato":                                     "sin formato",
		"<p>Uno</p><p>Dos &amp; tres</p>":                 "Uno\nDos & tres",
		"<ul><li>pan</li><li>leche</li></ul>":             "• pan\n• leche",
		"a<br>b<br/>c":                                    "a\nb\nc",
		"<h2>Título</h2><div>cuerpo&nbsp;aquí</div>":      "Título\ncuerpo aquí",
		"<p>hola</p><script>alert(1)</script><p>x</p>":    "hola\nx",
		"<style>p{color:red}</style>visible":              "visible",
		"<p>  muchos    espacios  </p>\n\n\n\n<p>fin</p>": "muchos espacios\n\nfin",
	} {
		if got := Text(html); got != quiero {
			t.Errorf("Text(%q) = %q, quiero %q", html, got, quiero)
		}
	}
	if HasMarkup("texto con 3 < 4 y nada más") || !HasMarkup("<b>negrita</b>") {
		t.Error("HasMarkup confunde un signo «menor que» con una etiqueta, o no ve una")
	}
	if got := Summary("<p>Uno</p><p>dos tres cuatro</p>", 9); got != "Uno dos t…" {
		t.Errorf("Summary = %q", got)
	}
}

// Raw y Escape son el `stripHtml`/`escapeHtml` de la hoja de cálculo de la web. Tienen
// que ser inversas y dar EXACTAMENTE lo que da la web: es lo que entra en el motor de
// fórmulas, y el mismo documento se abre en sytue.eu y en sytue.com.
func TestRawYEscapeSonLosDeLaHoja(t *testing.T) {
	for crudo, html := range map[string]string{
		`Mes`:           `Mes`,
		`a & b`:         `a &amp; b`,
		`<c>`:           `&lt;c&gt;`,
		`di "hola"`:     `di &quot;hola&quot;`,
		`=SUM(A1:A3)`:   `=SUM(A1:A3)`,
		`d'Artagnan`:    `d'Artagnan`, // el apóstrofo NO se escapa: html.EscapeString sí lo haría
		`&amp; literal`: `&amp;amp; literal`,
	} {
		if got := Escape(crudo); got != html {
			t.Errorf("Escape(%q) = %q, quiero %q", crudo, got, html)
		}
		if got := Raw(html); got != crudo {
			t.Errorf("Raw(%q) = %q, quiero %q", html, got, crudo)
		}
	}
	// Con formato en línea, Raw devuelve solo el texto; y un <br> en una celda no es un
	// salto de línea (una celda es una línea), que es en lo que se separa de Text.
	if got := Raw("<b>Mes</b>"); got != "Mes" {
		t.Errorf("Raw(<b>Mes</b>) = %q", got)
	}
	if got, texto := Raw("a<br>b"), Text("a<br>b"); got != "ab" || texto != "a\nb" {
		t.Errorf("Raw = %q y Text = %q: en una celda el <br> no es un salto", got, texto)
	}
	// &nbsp; es un espacio, y `&amp;lt;` vuelve como el TEXTO «&lt;», no como «<».
	if got := Raw("a&nbsp;b"); got != "a b" {
		t.Errorf("Raw(a&nbsp;b) = %q", got)
	}
	if got := Raw("&amp;lt;"); got != "&lt;" {
		t.Errorf("Raw(&amp;lt;) = %q, quiero &lt;", got)
	}
}
