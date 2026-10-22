// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

// Un enlace que viene de fuera solo va a un `href` si su destino es inofensivo: el HTML que
// sale del importador acaba en el DOM de quien abre el documento.
func TestEnlaceSeguro(t *testing.T) {
	for destino, quiere := range map[string]bool{
		"https://sytue.eu/a?b=1": true, "http://example.org": true, "HTTPS://EXAMPLE.ORG": true,
		"mailto:a@b.eu": true, "tel:+34600000000": true,
		"#fn-1": true, "/ruta/relativa": true, "./pagina.html": true, "//example.org/x": true, "": true,
		// Una ruta suelta es un fichero vecino del original: el saneador no la admite.
		"pagina.html": false, "Hyperlink%20-%20external.docx": false,
		"javascript:alert(1)": false, "JaVaScRiPt:alert(1)": false, " javascript:alert(1)": false,
		"\t javascript:alert(1)": false, "java\tscript:alert(1)": false, "java\nscript:alert(1)": false,
		"java\rscript:alert(1)": false, "\x01javascript:alert(1)": false, "java\x00script:alert(1)": false,
		"javascript\x7f:alert(1)": false, "javascript://example.org/%0aalert(1)": false,
		"data:text/html,<script>alert(1)</script>": false, "DATA:text/html;base64,PHNjcmlwdD4=": false,
		"vbscript:msgbox(1)": false, "VBScript:x": false, "file:///etc/passwd": false,
		"blob:https://example.org/x": false, "ftp://example.org": false, "x-otra:cosa": false,
		"http://[::1": false, // no se entiende: no va a un href
	} {
		if got := enlaceSeguro(destino); got != quiere {
			t.Errorf("enlaceSeguro(%q) = %v, quiere %v", destino, got, quiere)
		}
	}
}

// Lo mismo por la puerta de verdad: el importador de HTML, con el destino escrito como lo
// escribiría quien quiere colarlo (mayúsculas, espacios, saltos y entidades, que el
// analizador ya entrega decodificadas). El texto del enlace se conserva; el enlace, no.
func TestImportarHTMLNoDejaEnlacesEjecutables(t *testing.T) {
	hostiles := []string{
		`javascript:alert(1)`, `JAVASCRIPT:alert(1)`, `  javascript:alert(1)`, `&#106;avascript:alert(1)`,
		`&#x6A;avascript:alert(1)`, `javascript&colon;alert(1)`, `java&Tab;script:alert(1)`,
		`java&NewLine;script:alert(1)`, `java&#9;script:alert(1)`, `&#1;javascript:alert(1)`,
		`data:text/html,&lt;script&gt;alert(1)&lt;/script&gt;`, `vbscript:msgbox(1)`, `VbScript:x`,
		`file:///etc/passwd`,
	}
	for _, h := range hostiles {
		res, err := Import("x.html", []byte(`<p>antes <a href="`+h+`">pulsa</a> despues</p>`))
		if err != nil {
			t.Fatalf("%s: %v", h, err)
		}
		c := strings.ToLower(res.Content)
		if strings.Contains(c, "<a ") || strings.Contains(c, "href") || strings.Contains(c, "script:") || strings.Contains(c, "data:") {
			t.Errorf("%s: el enlace sobrevivió: %s", h, res.Content)
		}
		if !strings.Contains(res.Content, "pulsa") || !strings.Contains(res.Content, "antes") {
			t.Errorf("%s: se perdió el texto: %s", h, res.Content)
		}
	}
	// Los enlaces de siempre siguen entrando.
	res, err := Import("x.html", []byte(`<p><a href="https://sytue.eu/a?b=1&amp;c=2">web</a> <a href="mailto:a@b.eu">correo</a> <a href="#nota">ancla</a></p>`))
	if err != nil || !strings.Contains(res.Content, `<a href="https://sytue.eu/a?b=1&amp;c=2">web</a>`) ||
		!strings.Contains(res.Content, `<a href="mailto:a@b.eu">correo</a>`) || !strings.Contains(res.Content, `<a href="#nota">ancla</a>`) {
		t.Errorf("los enlaces buenos deben conservarse: %v %s", err, res.Content)
	}
}
