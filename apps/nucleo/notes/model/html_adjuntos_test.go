// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// LAS IMÁGENES, LOS FICHEROS Y LAS CELDAS COMBINADAS DEL TEXTO, AL GUARDAR.
//
// Lo que tiene que salir: una imagen de un adjunto de la nota (`adjunto:ID`) o pequeña y
// en línea, un enlace a un adjunto y `colspan`/`rowspan` con un número de verdad. Y lo
// que importa tanto como eso, lo que NO: cualquier manejador, cualquier dirección (de
// fuera o del propio servicio), un SVG, un `data:` que no es una imagen o es demasiado
// grande, un id de adjunto que se sale de su forma y un número que no es un número.
func TestSanitizeHTMLImagenesYFicheros(t *testing.T) {
	pequena := "data:image/png;base64," + strings.Repeat("A", 100)
	casos := []struct{ nombre, entrada, quiero string }{
		{"imagen de un adjunto", `<p><img src="adjunto:1732540000000-ab12cd" alt="plano.png"></p>`, `<p><img src="adjunto:1732540000000-ab12cd" alt="plano.png"></p>`},
		{"con medidas", `<img src="adjunto:a1" width="320" height="200" alt="x">`, `<img src="adjunto:a1" alt="x" width="320" height="200">`},
		{"pequeña y en línea", `<img src="` + pequena + `">`, `<img src="` + pequena + `">`},
		{"un fichero", `<a href="adjunto:a1">informe.pdf</a>`, `<a href="adjunto:a1" target="_blank" rel="noopener noreferrer">informe.pdf</a>`},
		{"el alt va escapado", `<img src="adjunto:a1" alt="&quot;><script>alert(1)</script>">`, `<img src="adjunto:a1" alt="&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;">`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SanitizeHTML(c.entrada); got != c.quiero {
				t.Errorf("SanitizeHTML(%q)\n  = %q\n  quiero %q", c.entrada, got, c.quiero)
			}
		})
	}
}

func TestSanitizeHTMLImagenesQueNoSalen(t *testing.T) {
	grande := "data:image/png;base64," + strings.Repeat("A", MaxImagenEnLinea)
	casos := []struct{ nombre, entrada, quiero string }{
		{"onerror", `<p><img src=x onerror=alert(1)>a</p>`, `<p>a</p>`},
		{"onerror con src válido", `<img src="adjunto:a1" onerror="alert(1)" onload="alert(2)">`, `<img src="adjunto:a1">`},
		{"javascript", `<img src="javascript:alert(1)">`, ``},
		{"javascript en mayúsculas y con blancos", `<img src=" JaVaScRiPt:alert(1)">`, ``},
		{"una dirección de fuera", `<img src="https://espia.example/pixel.gif">`, ``},
		{"sin esquema", `<img src="//espia.example/pixel.gif">`, ``},
		{"una ruta del propio servicio (GET con la sesión de quien mira)", `<img src="/api/auth/logout">`, ``},
		{"una ruta relativa", `<img src="../api/x">`, ``},
		{"blob", `<img src="blob:https://notes.example/1234">`, ``},
		{"SVG en línea", `<img src="data:image/svg+xml;base64,PHN2Zy8+">`, ``},
		{"HTML en línea", `<img src="data:text/html;base64,PHNjcmlwdD4=">`, ``},
		{"data sin base64", `<img src="data:image/png,%3Csvg%3E">`, ``},
		{"data en mayúsculas", `<img src="DATA:IMAGE/PNG;BASE64,AAAA">`, ``},
		{"demasiado grande", `<img src="` + grande + `">`, ``},
		{"id de adjunto que se sale", `<img src="adjunto:../../x">`, ``},
		{"id de adjunto con comillas", `<img src='adjunto:a1" onerror="alert(1)'>`, ``},
		{"adjunto con blancos", `<img src=" adjunto:a1">`, ``},
		{"srcset", `<img src="adjunto:a1" srcset="https://espia.example/x.png 2x">`, `<img src="adjunto:a1">`},
		{"style", `<img src="adjunto:a1" style="background:url(https://espia.example)">`, `<img src="adjunto:a1">`},
		{"medidas que no son números", `<img src="adjunto:a1" width="100%" height="1e3">`, `<img src="adjunto:a1">`},
		{"medidas fuera de tope", `<img src="adjunto:a1" width="99999" height="0">`, `<img src="adjunto:a1">`},
		{"src repetido: vale el primero", `<img src="https://espia.example/x" src="adjunto:a1">`, ``},
		{"alt larguísimo", `<img src="adjunto:a1" alt="` + strings.Repeat("a", MaxAltImagen+1) + `">`, `<img src="adjunto:a1">`},
		{"imagen dentro de un script", `<script><img src="adjunto:a1"></script>`, ``},
		{"un enlace a un adjunto falso", `<a href="adjunto:x/../y">a</a>`, `<a>a</a>`},
		{"un enlace javascript sigue fuera", `<a href="javascript:alert(1)">a</a>`, `<a>a</a>`},
		{"un cierre de img no abre nada", `<p>a</img>b</p>`, `<p>ab</p>`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SanitizeHTML(c.entrada); got != c.quiero {
				t.Errorf("SanitizeHTML(%q)\n  = %.200q\n  quiero %q", c.entrada, got, c.quiero)
			}
		})
	}
}

func TestSanitizeHTMLCeldasCombinadas(t *testing.T) {
	casos := []struct{ nombre, entrada, quiero string }{
		{"combinadas", `<td colspan="2" rowspan="3">a</td>`, `<td colspan="2" rowspan="3">a</td>`},
		{"cabecera combinada", `<th colspan="20">a</th>`, `<th colspan="20">a</th>`},
		{"con fondo", `<td colspan="2" data-fondo="azul">a</td>`, `<td colspan="2" data-fondo="azul">a</td>`},
		{"un 1 es no ponerlo", `<td colspan="1">a</td>`, `<td>a</td>`},
		{"cero", `<td rowspan="0">a</td>`, `<td>a</td>`},
		{"pasado de tope", `<td colspan="21" rowspan="101">a</td>`, `<td>a</td>`},
		{"con ceros delante", `<td colspan="02">a</td>`, `<td>a</td>`},
		{"con signo", `<td colspan="+2">a</td>`, `<td>a</td>`},
		{"no es un número", `<td colspan="2 onmouseover=alert(1)">a</td>`, `<td>a</td>`},
		{"fuera de una celda", `<p colspan="2">a</p>`, `<p>a</p>`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SanitizeHTML(c.entrada); got != c.quiero {
				t.Errorf("SanitizeHTML(%q)\n  = %q\n  quiero %q", c.entrada, got, c.quiero)
			}
		})
	}
}

// LA IDA Y VUELTA ENTRE LA WEB Y EL ESCRITORIO, en ficheros que leen las tres partes:
// `testdata/ida-y-vuelta/*.html` es el HTML de los bloques nuevos tal como lo escriben el
// editor de la web (el `innerHTML` del navegador) y el de Fyne. Aquí se comprueba que el
// saneador del servicio lo deja entero —como HTML: él escribe `open` y el navegador
// `open=""`, que son lo mismo—; Fyne comprueba que lo lee, lo vuelve a escribir byte a
// byte y lo puede guardar (`apps/fyne/apps/notes`), y la web, que el navegador lo
// conserva igual al entregarlo (`ida-y-vuelta.test.ts` de Notes).
func TestSanitizeHTMLIdaYVueltaDeLaWebYFyne(t *testing.T) {
	ficheros, err := filepath.Glob(filepath.Join("testdata", "ida-y-vuelta", "*.html"))
	if err != nil || len(ficheros) < 5 {
		t.Fatalf("faltan los ficheros de ida y vuelta: %v %v", ficheros, err)
	}
	for _, f := range ficheros {
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			entrada := strings.TrimSpace(string(b))
			if got := normalizarHTML(t, SanitizeHTML(entrada)); got != normalizarHTML(t, entrada) {
				t.Errorf("el saneador cambia %s:\n  %s\n  %s", f, got, normalizarHTML(t, entrada))
			}
		})
	}
}

// normalizarHTML analiza y vuelve a escribir un trozo de HTML: lo que compara el
// navegador, sin las diferencias de forma (`open` y `open=""`, `<br>` y `<br/>`).
func normalizarHTML(t *testing.T, s string) string {
	t.Helper()
	nodos, err := html.ParseFragment(strings.NewReader(s), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, n := range nodos {
		if err := html.Render(&b, n); err != nil {
			t.Fatal(err)
		}
	}
	return b.String()
}
