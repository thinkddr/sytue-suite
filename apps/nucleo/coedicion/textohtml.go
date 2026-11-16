// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package coedicion

import (
	"strconv"
	"strings"
)

// El puente de `editor/state/textohtml.ts` entre el HTML del documento (lo que lleva la
// secuencia, un elemento por carácter del HTML) y el texto que ve quien escribe, en el que
// están medidos los offsets de un cursor. Sirve para anclar el cursor a la identidad de un
// carácter en vez de a un número que cualquier edición ajena desplaza.
//
// Los offsets de texto van en unidades UTF-16, como `textContent` en la web: es lo que
// viaja en el cursor y lo que lee el otro lado. Un emoji cuenta dos.
//
// Es un lector de HTML serializado (etiquetas `<…>` con comillas en los atributos,
// comentarios y referencias `&…;`), no un parser general. Cuando no coincide con lo que se
// ve, quien lo usa lo detecta por la longitud y vuelve a los offsets: nunca ancla a ciegas.

// unidades da cuántas unidades UTF-16 de texto aporta cada code point del HTML (0 =
// marcado). Es `unidades` de la web paso a paso: cambiar un caso aquí y no allí hace que
// cada lado coloque el cursor en un carácter distinto.
func unidades(html string) []int {
	cps := []rune(html)
	texto := make([]int, len(cps))
	i := 0
	for i < len(cps) {
		c := cps[i]
		if c == '<' {
			i = finDeMarcado(cps, i)
			continue
		}
		if c == '&' {
			if j, u := referencia(cps, i); u > 0 {
				texto[j] = u // el texto se cuenta en el último carácter de la referencia
				i = j + 1
				continue
			}
		}
		texto[i] = unidadesDe(c)
		i++
	}
	return texto
}

// unidadesDe es `.length` de un code point: 1, o 2 fuera del plano básico.
func unidadesDe(r rune) int {
	if r > 0xffff {
		return 2
	}
	return 1
}

// finDeMarcado es dónde sigue el HTML tras el comentario o la etiqueta que empieza en
// `i`: tras el '>' que la cierra, sin contar los que van dentro de comillas.
func finDeMarcado(cps []rune, i int) int {
	if i+3 < len(cps) && cps[i+1] == '!' && cps[i+2] == '-' && cps[i+3] == '-' {
		j := i + 4
		for j < len(cps) && (cps[j] != '>' || cps[j-1] != '-' || cps[j-2] != '-') {
			j++
		}
		return j + 1
	}
	j := i + 1
	var comilla rune
	for ; j < len(cps); j++ {
		d := cps[j]
		switch {
		case comilla != 0:
			if d == comilla {
				comilla = 0
			}
		case d == '"' || d == '\'':
			comilla = d
		case d == '>':
			return j + 1
		}
	}
	return j + 1
}

// referencia lee la referencia `&…;` que empieza en `i`: dónde está su ';' y cuántas
// unidades de texto aporta, o 0 si no es una referencia (y el '&' cuenta como texto).
func referencia(cps []rune, i int) (int, int) {
	j := i + 1
	for j < len(cps) && j-i <= 12 && esAlfanumericoRef(cps[j]) {
		j++
	}
	if j >= len(cps) || cps[j] != ';' || j == i+1 {
		return 0, 0
	}
	if valorReferencia(string(cps[i+1:j])) > 0xffff {
		return j, 2
	}
	return j, 1
}

func esAlfanumericoRef(r rune) bool {
	return r == '#' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// valorReferencia es el code point de una referencia numérica (`#123`, `#x1F600`) o 0 si
// es una con nombre (`amp`) o no se lee: entonces cuenta como una unidad.
func valorReferencia(ref string) uint64 {
	var cifras string
	base := 10
	switch {
	case len(ref) > 2 && (ref[:2] == "#x" || ref[:2] == "#X"):
		cifras, base = ref[2:], 16
	case len(ref) > 1 && ref[0] == '#':
		cifras = ref[1:]
	default:
		return 0
	}
	validas := "0123456789"
	if base == 16 {
		validas = "0123456789abcdefABCDEF"
	}
	if strings.Trim(cifras, validas) != "" {
		return 0
	}
	n, err := strconv.ParseUint(cifras, base, 64)
	if err != nil {
		return 0x10ffff + 1 // no cabe: en la web también pasa del plano básico
	}
	return n
}

// LongitudVisible es `longitudVisible`: cuántas unidades de texto tiene el HTML, para
// saber si la secuencia dice lo mismo que el folio.
func LongitudVisible(html string) int {
	n := 0
	for _, u := range unidades(html) {
		n += u
	}
	return n
}

// IndiceHTMLDeTexto es `indiceHtmlDeTexto`: tras cuántos code points del HTML se han
// consumido `offTexto` unidades de texto (justo tras el último carácter visible, antes del
// marcado que siga). false a mitad de un par suplente o más allá del final.
func IndiceHTMLDeTexto(html string, offTexto int) (int, bool) {
	if offTexto <= 0 {
		return 0, true
	}
	visto := 0
	for i, u := range unidades(html) {
		visto += u
		if visto == offTexto {
			return i + 1, true
		}
		if visto > offTexto {
			return 0, false
		}
	}
	return 0, false
}

// OffsetDeIndiceHTML es `offsetDeIndiceHtml`: cuántas unidades de texto hay en los
// primeros `indice` code points del HTML.
func OffsetDeIndiceHTML(html string, indice int) int {
	visto := 0
	for i, u := range unidades(html) {
		if i >= indice {
			break
		}
		visto += u
	}
	return visto
}
