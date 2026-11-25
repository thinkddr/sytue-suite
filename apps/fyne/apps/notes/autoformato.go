// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"unicode"
)

// EL FORMATO QUE SE ESCRIBE: los atajos de Markdown al teclear.
//
// De bloque, al principio de la línea y con un espacio detrás (`atajoMarkdown`, el
// `client/markdown.ts` de la web): «# »…«##### » títulos, «- », «* » y «+ » viñetas,
// «1. » números, «[] » o «[ ] » una tarea, «> » una cita y «``` » un bloque de código; y
// «---» con Intro, un separador.
//
// En línea, al CERRAR la marca: `**x**` negrita, `*x*` y `_x_` cursiva, `` `x` `` código
// y `~~x~~` tachado. Se escriben con las MISMAS marcas que la barra (`<b>`, `<i>`,
// `<code>`, `<s>`), así que no hay nada nuevo que guardar, sanear ni leer en la web. Lo
// que se escribe después ya va sin la marca, como en cualquier editor que lo hace.
//
// Las dos cosas son su PROPIO paso de deshacer, detrás de la letra que las dispara: Ctrl/⌘+Z
// justo después devuelve el texto tal cual se escribió («## », «**hola**») en vez de
// borrarlo, que es lo que se quiere cuando el atajo no era un atajo. Nunca dentro de un
// bloque de código ni de un código en línea: ahí las marcas son texto.

// marcaEnLinea dice si las runas `r` (lo que hay antes del cursor en su bloque) acaban en
// una marca de Markdown recién cerrada: qué marca es (la de `AlternarMarca`), dónde empieza
// su apertura y cuánto mide cada uno de sus dos delimitadores.
func marcaEnLinea(r []rune) (marca string, apertura, delim int, ok bool) {
	n := len(r)
	if n < 3 {
		return "", 0, 0, false
	}
	switch r[n-1] {
	case '*':
		if r[n-2] == '*' {
			if a, ok := apertura2(r, '*'); ok {
				return "b", a, 2, true
			}
			return "", 0, 0, false
		}
		if a, ok := apertura1(r, '*', false); ok {
			return "i", a, 1, true
		}
	case '_':
		if a, ok := apertura1(r, '_', false); ok {
			return "i", a, 1, true
		}
	case '~':
		if r[n-2] == '~' {
			if a, ok := apertura2(r, '~'); ok {
				return "s", a, 2, true
			}
		}
	case '`':
		if a, ok := apertura1(r, '`', true); ok {
			return "code", a, 1, true
		}
	}
	return "", 0, 0, false
}

// apertura1 busca la apertura de una marca de UNA letra cerrada al final de `r`: la más
// cercana hacia atrás. El contenido no puede estar vacío ni empezar o acabar en blanco
// (salvo el código, que admite blancos dentro); la apertura no puede ir pegada a otra
// igual («**» no es una cursiva vacía) ni, en `*` y `_`, detrás de una letra («2*3*»,
// «snake_case_»).
func apertura1(r []rune, d rune, codigo bool) (int, bool) {
	fin := len(r) - 1 // donde está el cierre
	if fin < 2 || r[fin-1] == d || (!codigo && unicode.IsSpace(r[fin-1])) {
		return 0, false
	}
	for k := fin - 1; k >= 0; k-- {
		switch {
		case r[k] == '\n':
			return 0, false
		case r[k] != d:
			continue
		}
		contenido := r[k+1 : fin]
		if len(contenido) == 0 || (!codigo && unicode.IsSpace(contenido[0])) || !conLetra(contenido) {
			return 0, false
		}
		if k > 0 && (r[k-1] == d || (!codigo && esDePalabra(r[k-1]))) {
			return 0, false
		}
		return k, true
	}
	return 0, false
}

// apertura2 es apertura1 para las marcas dobles («**», «~~»): el contenido puede llevar
// la letra suelta («**a*b**»), pero no la pareja.
func apertura2(r []rune, d rune) (int, bool) {
	fin := len(r) - 2
	if fin < 3 || r[fin-1] == d || unicode.IsSpace(r[fin-1]) {
		return 0, false
	}
	for k := fin - 1; k >= 1; k-- {
		if r[k] == '\n' {
			return 0, false
		}
		if r[k] != d || r[k-1] != d {
			continue
		}
		contenido := r[k+1 : fin]
		if len(contenido) == 0 || unicode.IsSpace(contenido[0]) || contenido[0] == d || !conLetra(contenido) {
			return 0, false
		}
		if k >= 2 && r[k-2] == d {
			return 0, false
		}
		return k - 1, true
	}
	return 0, false
}

func conLetra(rs []rune) bool {
	return slices.ContainsFunc(rs, func(r rune) bool { return !unicode.IsSpace(r) })
}

func esDePalabra(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }

// autoformatoEnLinea mira, tras escribir una letra de marca, si con ella se ha cerrado una
// de Markdown; si es así, quita los delimitadores y pone la marca en lo de dentro, en su
// propio paso de deshacer. Dice si lo ha hecho.
func (e *EditorNota) autoformatoEnLinea() bool {
	if !e.AutoformatoEnLinea || e.haySeleccion() || len(e.doc.Bloques) == 0 {
		return false
	}
	b := e.bloqueActual()
	if b.Tipo == Codigo || !editable(b) || tieneMarca(estiloEn(b, e.cur.O), "code") {
		return false
	}
	r := []rune(b.Texto())
	o := min(e.cur.O, len(r))
	desde := inicioEditable(b)
	marca, ap, delim, ok := marcaEnLinea(r[desde:o])
	if !ok {
		return false
	}
	ap += desde
	// Lo que hay dentro no puede tener ya código en línea: sus marcas son texto.
	for k := ap + delim; k < o-delim; k++ {
		if tieneMarca(estiloEn(b, k+1), "code") {
			return false
		}
	}
	bl := e.cur.B
	e.hacer(func() {
		e.doc.Borrar(Pos{bl, o - delim}, Pos{bl, o})
		e.doc.Borrar(Pos{bl, ap}, Pos{bl, ap + delim})
		fin := o - 2*delim
		e.doc.tramos(Pos{bl, ap}, Pos{bl, fin}, func(t *Tramo) { *t = conMarca(*t, marca, true) })
		e.cur = Pos{bl, fin}
		e.ancla = e.cur
	})
	// Lo que se escriba ahora ya va sin la marca.
	t := conMarca(estiloEn(e.bloqueActual(), e.cur.O), marca, false)
	e.pendiente = &t
	e.refrescarBarra()
	return true
}

// esLetraDeMarca dice si una letra puede cerrar una marca en línea.
func esLetraDeMarca(r rune) bool { return r == '*' || r == '_' || r == '`' || r == '~' }
