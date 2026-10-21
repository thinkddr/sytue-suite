// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// HASHTAGS
//
// Un hashtag es `#palabra` escrita en el título o en el texto de una nota. No es
// la «etiqueta con estado» de tags.go —esa se elige de una lista y cambia el
// aspecto de la nota—: aquí no hay nada que crear ni que mantener, se escribe y
// ya existe.
//
// HAY UN SOLO EXTRACTOR, y es este. La lista de hashtags con su recuento y la
// búsqueda por `#palabra` leen los dos de aquí; con una expresión en SQL para una
// cosa y otra en Go para la otra, el recuento diría 3 y la búsqueda enseñaría 2.

// reHashtag pide que el `#` EMPIECE palabra y que lo siga una letra:
//   - `texto#ancla` y `https://sitio/#ancla` no son hashtags (el `#` va pegado a
//     una letra o a una barra);
//   - `#1` y `&#39;` tampoco (empiezan por cifra: son un número de orden o una
//     entidad que alguien dejó sin resolver);
//   - `##` es un título de Markdown pegado, no una etiqueta.
var reHashtag = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_&/#])#(\p{L}[\p{L}\p{N}_-]*)`)

// maxHashtag es lo que mide como mucho un hashtag, en letras. Más largo no es una
// etiqueta: es una línea pegada sin espacios.
const maxHashtag = 64

// Hashtags devuelve los hashtags de un TEXTO LLANO, en orden y tal como se
// escribieron (sin el `#`, con repetidos). Quien cuenta o compara los pliega.
func Hashtags(texto string) []string {
	var out []string
	for _, m := range reHashtag.FindAllStringSubmatch(texto, -1) {
		// `#plan-` al final de una frase partida: el guion no es de la etiqueta.
		tag := strings.TrimRight(m[1], "-_")
		if utf8.RuneCountInString(tag) > maxHashtag || esColor(tag) {
			continue
		}
		out = append(out, tag)
	}
	return out
}

// esColor aparta `#ff0000`: seis hexadecimales con alguna cifra son un color que
// alguien anotó, no una etiqueta. `#decade` (sin cifras) sí lo es.
//
// ponytail: solo #RRGGBB. `#f00` y `#b2b` miden lo mismo y el segundo es una
// etiqueta de verdad; si los colores cortos molestan, se distinguen por contexto.
func esColor(tag string) bool {
	return ValidHexColor("#"+tag) && strings.ContainsAny(tag, "0123456789")
}

// HashtagDeConsulta dice si lo buscado es EXACTAMENTE un hashtag («#plan»), y cuál.
// Es lo que separa «notas con esta etiqueta» de una búsqueda de texto corriente.
func HashtagDeConsulta(q string) (string, bool) {
	q = strings.TrimSpace(q)
	tags := Hashtags(q)
	if len(tags) != 1 || "#"+tags[0] != q {
		return "", false
	}
	return tags[0], true
}
