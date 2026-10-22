// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// AMPLÍA la cobertura del saneado, que ya tenía ocho casos en `domain_test.go`
// (script, onclick, `javascript:`, enlace con noopener, style, iframe y los dos
// de balanceo). Lo que faltaba eran los vectores vecinos de esos mismos —los que
// se cuelan justo cuando uno cree que el caso está cubierto— y la otra mitad del
// contrato: que lo PERMITIDO siga saliendo.
//
// Importa porque esto decide qué HTML de una persona acaba pintado en el
// navegador de otra —un cuaderno de organización, un enlace público— y es el
// sitio donde un fallo no se ve al usarlo: la nota se ve bien mientras el
// `onerror` se ejecuta en silencio.
//
// Va aparte y no dentro de `domain_test.go` porque este saneador es un fichero
// propio (`html.go`): quien lo toque busca sus pruebas al lado.

// Vecinos de los que ya estaban. `svg` y `style` completan la lista de subárboles
// que se tiran enteros; `onerror` es el manejador que se pone cuando `onclick`
// no cuela; `data:` es el esquema que queda cuando se filtra `javascript:`.
func TestSanitizeHTMLVectoresVecinos(t *testing.T) {
	casos := []struct{ nombre, entrada, quiero string }{
		{"style se descarta con su contenido", `<style>p{x:1}</style><p>a</p>`, `<p>a</p>`},
		{"svg se descarta entero", `<svg onload="alert(1)"><circle/></svg><p>a</p>`, `<p>a</p>`},
		{"onerror tampoco sobrevive", `<p onerror="alert(1)">a</p>`, `<p>a</p>`},
		{"data: en href se cae", `<a href="data:text/html,x">x</a>`, `<a>x</a>`},
		// La etiqueta desconocida se va; su texto NO. Perderlo sería perder la nota.
		{"etiqueta desconocida conserva su texto", `<marquee>hola</marquee>`, `hola`},
		// Lo que llega como texto no puede volver a ser marcado al pintarlo.
		{"el texto se escapa", `<p>2 < 3</p>`, `<p>2 &lt; 3</p>`},
		{"comentario fuera", `<p>a</p><!-- oculto -->`, `<p>a</p>`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SanitizeHTML(c.entrada); got != c.quiero {
				t.Errorf("SanitizeHTML(%q)\n  = %q\n  quiero %q", c.entrada, got, c.quiero)
			}
		})
	}
}

// La otra mitad del contrato. Sin esto, «saneado» y «vacío» pasan los mismos
// tests: un saneador que devolviera cadena vacía siempre sería PERFECTO contra
// XSS e inútil como editor.
func TestSanitizeHTMLConservaLoPermitido(t *testing.T) {
	casos := []struct{ nombre, entrada, quiero string }{
		{"formato básico", `<p><b>a</b> <i>b</i></p>`, `<p><b>a</b> <i>b</i></p>`},
		{"listas", `<ul><li>a</li><li>b</li></ul>`, `<ul><li>a</li><li>b</li></ul>`},
		{"encabezado y cita", `<h2>t</h2><blockquote>c</blockquote>`, `<h2>t</h2><blockquote>c</blockquote>`},
		{"salto de línea", `a<br>b`, `a<br>b`},
		{
			"mailto es tan válido como https",
			`<a href="mailto:a@b.com">x</a>`,
			`<a href="mailto:a@b.com" target="_blank" rel="noopener noreferrer">x</a>`,
		},
		{
			"tareas con casilla interactiva",
			`<div class="task-row"><input type="checkbox" checked> <span>Hacer la compra</span></div>`,
			`<div class="task-row"><input type="checkbox" checked> <span>Hacer la compra</span></div>`,
		},
		{
			"lista plegada",
			`<ul><li class="plegada">a<ul><li>b</li></ul></li></ul>`,
			`<ul><li class="plegada">a<ul><li>b</li></ul></li></ul>`,
		},
		// `plegada` solo vale en un elemento de lista.
		{"plegada fuera de una lista", `<p class="plegada">a</p>`, `<p>a</p>`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SanitizeHTML(c.entrada); got != c.quiero {
				t.Errorf("SanitizeHTML(%q)\n  = %q\n  quiero %q", c.entrada, got, c.quiero)
			}
		})
	}
}

// Balanceo: los dos que ya estaban cubren abrir sin cerrar y el cierre huérfano.
// Falta el cruce —abierto por dentro, cerrado por fuera— que es el que produce
// el HTML que un navegador «arregla» tragándose lo que viene detrás.
func TestSanitizeHTMLCierraLoAnidado(t *testing.T) {
	casos := []struct{ nombre, entrada, quiero string }{
		{"anidado mal cerrado", `<p><b>a</p>`, `<p><b>a</b></p>`},
		{"vacío", ``, ``},
		{"solo espacios", `   `, ``},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SanitizeHTML(c.entrada); got != c.quiero {
				t.Errorf("SanitizeHTML(%q)\n  = %q\n  quiero %q", c.entrada, got, c.quiero)
			}
		})
	}
}

// La barra del editor ofrece títulos hasta h5 y alineación del bloque; el
// saneador tiene que dejarlos pasar, y solo con los valores que la barra
// produce: `text-align` no es una puerta para `url(...)` ni para nada más.
func TestSanitizeHTMLTitulosYAlineacion(t *testing.T) {
	casos := []struct{ nombre, entrada, quiero string }{
		{"h5 y h6 sobreviven", `<h5>a</h5><h6>b</h6>`, `<h5>a</h5><h6>b</h6>`},
		{"alineación válida", `<p style="text-align: center">a</p>`, `<p style="text-align:center">a</p>`},
		{"alineación en mayúsculas se normaliza", `<p style="TEXT-ALIGN: Right">a</p>`, `<p style="text-align:right">a</p>`},
		{"alineación inventada se cae", `<p style="text-align: url(x)">a</p>`, `<p>a</p>`},
		{"color y alineación juntos", `<p style="color:#112233;text-align:justify">a</p>`, `<p style="color:#112233;text-align:justify">a</p>`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SanitizeHTML(c.entrada); got != c.quiero {
				t.Errorf("SanitizeHTML(%q)\n  = %q\n  quiero %q", c.entrada, got, c.quiero)
			}
		})
	}
}
