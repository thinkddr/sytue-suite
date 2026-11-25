// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import "testing"

// LOS BLOQUES DEL EDITOR SOBREVIVEN AL GUARDAR, Y SOLO CON SUS VALORES.
//
// Panel, sección que se pliega, estado, decisiones, fecha, lenguaje del código, tabla
// con cabeceras y fondos, y columnas: el HTML que escriben la web y Fyne. Lo que
// importa tanto como que salga es que un valor fuera de su lista NO salga —un
// `data-panel="<script>"` o un `datetime` que no es una fecha—, y que un atributo de
// un bloque no valga en otra etiqueta.
func TestSanitizeHTMLBloquesDelEditor(t *testing.T) {
	casos := []struct{ nombre, entrada, quiero string }{
		{"panel", `<div data-panel="aviso"><p>Ojo</p></div>`, `<div data-panel="aviso"><p>Ojo</p></div>`},
		{
			"sección que se pliega, abierta",
			`<details data-expandir open><summary>Más</summary><p>x</p></details>`,
			`<details data-expandir open><summary>Más</summary><p>x</p></details>`,
		},
		{"estado", `<span data-estado="verde">HECHO</span>`, `<span data-estado="verde">HECHO</span>`},
		{
			"decisiones",
			`<ul data-tipo="decisiones"><li>Sí</li></ul>`,
			`<ul data-tipo="decisiones"><li>Sí</li></ul>`,
		},
		{
			"fecha",
			`<time datetime="2026-11-24">24 nov</time>`,
			`<time datetime="2026-11-24">24 nov</time>`,
		},
		{
			"código con lenguaje, números y ajuste",
			`<pre data-lineas data-ajuste><code data-lenguaje="go">x := 1</code></pre>`,
			`<pre data-lineas data-ajuste><code data-lenguaje="go">x := 1</code></pre>`,
		},
		{
			"código en TypeScript, con la grafía de Fyne",
			`<pre><code data-lenguaje="typescript">let x</code></pre>`,
			`<pre><code data-lenguaje="typescript">let x</code></pre>`,
		},
		{
			"tabla con cabeceras, fondo y filas numeradas",
			`<table data-numerada><thead><tr><th>A</th></tr></thead>` +
				`<tbody><tr><th scope="row" data-fondo="azul">1</th><td data-fondo="rojo">x</td></tr></tbody></table>`,
			`<table data-numerada><thead><tr><th>A</th></tr></thead>` +
				`<tbody><tr><th scope="row" data-fondo="azul">1</th><td data-fondo="rojo">x</td></tr></tbody></table>`,
		},
		{
			"columnas",
			`<div data-columnas="3"><div data-columna><p>a</p></div></div>`,
			`<div data-columnas="3"><div data-columna><p>a</p></div></div>`,
		},
		{"subíndice y superíndice", `<p>H<sub>2</sub>O x<sup>2</sup></p>`, `<p>H<sub>2</sub>O x<sup>2</sup></p>`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SanitizeHTML(c.entrada); got != c.quiero {
				t.Errorf("SanitizeHTML(%q)\n  = %q\n  quiero %q", c.entrada, got, c.quiero)
			}
		})
	}
}

func TestSanitizeHTMLBloquesConValoresAjenos(t *testing.T) {
	casos := []struct{ nombre, entrada, quiero string }{
		{"panel desconocido", `<div data-panel="peligro">a</div>`, `<div>a</div>`},
		{"panel con marcado", `<div data-panel="&quot;><script>">a</div>`, `<div>a</div>`},
		{"estado con un color crudo", `<span data-estado="#f00">a</span>`, `<span>a</span>`},
		{"estado fuera de un span", `<p data-estado="verde">a</p>`, `<p>a</p>`},
		{"tipo de lista desconocido", `<ul data-tipo="tareas"><li>a</li></ul>`, `<ul><li>a</li></ul>`},
		{"fecha que no es un día", `<time datetime="2026-02-30">a</time>`, `<time>a</time>`},
		{"fecha con hora", `<time datetime="2026-11-24T10:00">a</time>`, `<time>a</time>`},
		{"lenguaje desconocido", `<pre><code data-lenguaje="brainfuck">a</code></pre>`, `<pre><code>a</code></pre>`},
		{"un booleano con valor", `<details data-expandir="sí"><summary>a</summary></details>`, `<details><summary>a</summary></details>`},
		{"fondo con un color crudo", `<td data-fondo="red">a</td>`, `<td>a</td>`},
		{"scope desconocido", `<th scope="all">a</th>`, `<th>a</th>`},
		{"scope de columna: la cabecera es el <thead>", `<th scope="col">a</th>`, `<th>a</th>`},
		{"lenguaje con otra grafía", `<pre><code data-lenguaje="js">a</code></pre>`, `<pre><code>a</code></pre>`},
		{"columnas de más", `<div data-columnas="7">a</div>`, `<div>a</div>`},
		{"atributo repetido: vale el primero", `<div data-panel="info" data-panel="error">a</div>`, `<div data-panel="info">a</div>`},
		{"data-* que no es de un bloque", `<p data-x="1" data-panel="info">a</p>`, `<p>a</p>`},
		{"manejador en un bloque", `<details data-expandir ontoggle="alert(1)"><summary>a</summary></details>`, `<details data-expandir><summary>a</summary></details>`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SanitizeHTML(c.entrada); got != c.quiero {
				t.Errorf("SanitizeHTML(%q)\n  = %q\n  quiero %q", c.entrada, got, c.quiero)
			}
		})
	}
}
