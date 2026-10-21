// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package buscar es la regla ÚNICA de «¿coincide lo que escribo con lo que hay?» de la
// suite: «reunion» encuentra «reunión», «cafe» encuentra «Café», «Muller» encuentra
// «Müller», y al revés. Sin mayúsculas ni acentos.
//
// La usan los servicios (módulo raíz) y las apps de Fyne: por eso vive en `nucleo`, que
// importan los dos, y no en `platform/pkg`, que Fyne no importa. Tiene dos espejos que
// DEBEN decir lo mismo, y los tres leen los mismos casos de `testdata/casos.json`:
//
//   - `plegar` de '@sytue/i18n' en la web;
//   - la función SQL `plegar(text)` de cada base (extensión `unaccent`), para lo que
//     filtra el servidor.
package buscar

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// sueltas son las letras que NO se descomponen en base + marca y que aun así se leen
// como otra: la NFD deja «ß» o «ł» como están. Son las que `unaccent` también pliega.
var sueltas = strings.NewReplacer(
	"ß", "ss", "æ", "ae", "œ", "oe", "ø", "o", "ł", "l", "đ", "d", "ð", "d", "þ", "th", "ı", "i",
)

var sinMarcas = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// Plegar devuelve s en minúsculas y sin diacríticos. No recorta espacios: eso es cosa
// de quien lee la caja de búsqueda.
func Plegar(s string) string {
	if esASCII(s) {
		return strings.ToLower(s)
	}
	s = sueltas.Replace(strings.ToLower(s))
	out, _, err := transform.String(sinMarcas, s)
	if err != nil {
		return s
	}
	return out
}

// Contiene dice si aguja aparece en texto con la regla de Plegar. Una aguja vacía
// está en cualquier texto, como con strings.Contains.
func Contiene(texto, aguja string) bool {
	return strings.Contains(Plegar(texto), Plegar(aguja))
}

func esASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// Rangos dice DÓNDE está aguja dentro de pajar con la regla de Plegar: rangos
// [inicio, fin) en RUNAS del texto original, sin solaparse. Es `rangos` de
// '@sytue/i18n': para resaltar o sustituir lo encontrado, «reunion» marca «reunión»
// entera. Una marca suelta (tilde combinada) queda dentro del rango.
func Rangos(pajar []rune, aguja string) [][2]int {
	buscada := []rune(Plegar(aguja))
	if len(buscada) == 0 {
		return nil
	}
	// De cada runa plegada, la runa original de la que sale.
	plano := make([]rune, 0, len(pajar))
	origen := make([]int, 0, len(pajar)+1)
	for i, r := range pajar {
		if r < utf8.RuneSelf {
			plano = append(plano, unicode.ToLower(r))
			origen = append(origen, i)
			continue
		}
		for _, p := range Plegar(string(r)) {
			plano = append(plano, p)
			origen = append(origen, i)
		}
	}
	origen = append(origen, len(pajar))
	var out [][2]int
	for j := 0; j+len(buscada) <= len(plano); {
		if string(plano[j:j+len(buscada)]) != string(buscada) {
			j++
			continue
		}
		inicio, fin := origen[j], origen[j+len(buscada)]
		if fin <= inicio { // media letra plegada («s» dentro de «ß»): la letra entera
			fin = inicio + 1
		}
		// La otra mitad de esa misma letra ya está marcada.
		if n := len(out); n == 0 || inicio >= out[n-1][1] {
			out = append(out, [2]int{inicio, fin})
		}
		j += len(buscada)
	}
	return out
}

// Terminos son las palabras de una consulta de la caja de búsqueda, sin la sintaxis de
// `websearch_to_tsquery` que el servidor entiende y que no se resalta: comillas, el «-»
// de «sin esta palabra» y el «or». Es `terminos` de '@sytue/i18n'.
func Terminos(consulta string) []string {
	var out []string
	for _, p := range strings.Fields(consulta) {
		if strings.HasPrefix(p, "-") || strings.EqualFold(p, "or") {
			continue
		}
		if p = strings.Trim(p, `"`); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// RangosDeConsulta son los rangos de TODOS los términos de la consulta en pajar, en orden
// y fundidos donde se tocan: lo que se pinta resaltado en un resultado de búsqueda.
func RangosDeConsulta(pajar []rune, consulta string) [][2]int {
	terminos := Terminos(consulta)
	todos := make([][2]int, 0, len(terminos))
	for _, t := range terminos {
		todos = append(todos, Rangos(pajar, t)...)
	}
	slices.SortFunc(todos, func(a, b [2]int) int { return a[0] - b[0] })
	var out [][2]int
	for _, r := range todos {
		if n := len(out); n > 0 && r[0] <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], r[1])
			continue
		}
		out = append(out, r)
	}
	return out
}

// Fragmento es el trozo de texto de un resultado de búsqueda: unas `n` runas alrededor de
// la PRIMERA coincidencia de la consulta, y no las `n` primeras de la nota, que es lo que
// se enseñaba y casi nunca contenía lo buscado. Se corta en blanco cuando lo hay cerca y
// lleva «…» por el lado que se ha cortado. Sin coincidencia, el principio.
func Fragmento(texto, consulta string, n int) string {
	r := []rune(strings.Join(strings.Fields(texto), " "))
	if len(r) <= n {
		return string(r)
	}
	inicio := 0
	if rs := RangosDeConsulta(r, consulta); len(rs) > 0 {
		// Un tercio del hueco por delante: se lee de dónde viene la frase.
		inicio = max(0, rs[0][0]-n/3)
	}
	inicio = min(inicio, len(r)-n)
	if inicio > 0 {
		if i := slices.Index(r[inicio:min(inicio+15, len(r))], ' '); i >= 0 {
			inicio += i + 1
		}
	}
	fin := min(len(r), inicio+n)
	if fin < len(r) {
		if i := lastIndexRune(r[max(inicio, fin-15):fin], ' '); i >= 0 {
			fin = max(inicio, fin-15) + i
		}
	}
	s := string(r[inicio:fin])
	if inicio > 0 {
		s = "…" + s
	}
	if fin < len(r) {
		s += "…"
	}
	return s
}

func lastIndexRune(r []rune, c rune) int {
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] == c {
			return i
		}
	}
	return -1
}
