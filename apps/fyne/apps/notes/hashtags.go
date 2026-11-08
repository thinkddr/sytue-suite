// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"regexp"
	"slices"
	"strings"
)

// LOS #HASHTAGS DE UNA NOTA, PULSABLES AL PINTARLA: el espejo de `conHashtags` de la web
// (client/hashtags.ts). Quién es hashtag lo decide el servicio (`domain.Hashtags`): esto
// es la misma regla para pintar, y nada de lo que hace se guarda. Pulsar uno abre «Buscar
// notas» con `#palabra`, como en la web.

// reHashtag es `#` que empieza palabra y va seguido de letra: ni `texto#ancla`, ni
// `/#ancla`, ni `#1`, ni `&#39;`, ni el `##` de un título de Markdown.
var reHashtag = regexp.MustCompile(`(^|[^\p{L}\p{N}_&/#])#(\p{L}[\p{L}\p{N}_-]*)`)

// reColorHex es un `#ff0000`, que no es un hashtag aunque lo parezca.
var reColorHex = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)

// etiquetaDe es la etiqueta sin el guion de cola, o "" si es un color o no cabe.
func etiquetaDe(cruda string) string {
	tag := strings.TrimRight(cruda, "-_")
	if len([]rune(tag)) > 64 || reColorHex.MatchString(tag) && strings.ContainsAny(tag, "0123456789") {
		return ""
	}
	return tag
}

// hrefHashtag es el enlace que lleva un hashtag pintado: `#palabra`, como en la web.
func hrefHashtag(tag string) string { return "#" + tag }

// hashtagDe dice si un enlace es el de un hashtag pintado, y cuál.
func hashtagDe(href string) (string, bool) {
	tag, ok := strings.CutPrefix(href, "#")
	return tag, ok && tag != "" && etiquetaDe(tag) == tag && reHashtag.MatchString(href)
}

// conHashtags es una COPIA del documento con cada hashtag de su texto en un tramo con su
// enlace; el original no se toca, porque es el que se edita y se guarda. Dentro de un
// enlace, de `code` o de un bloque de código no se toca: ahí un `#` es de una URL o de un
// trozo de código. Los bloques siguen en sus índices (lo usa el plegado de las listas).
func conHashtags(d *Documento) *Documento {
	if d == nil || !hayHashtag(d) {
		return d
	}
	copia := *d
	copia.Bloques = slices.Clone(d.Bloques)
	for i, b := range copia.Bloques {
		if b.Tipo == Codigo || b.Tipo == BloqueOpaco {
			continue
		}
		var tramos []Tramo
		for _, t := range b.Tramos {
			tramos = append(tramos, partirHashtags(t)...)
		}
		copia.Bloques[i].Tramos = tramos
	}
	return &copia
}

// hayHashtag dice si algún bloque lleva uno: sin ninguno no hace falta copiar nada.
func hayHashtag(d *Documento) bool {
	for _, b := range d.Bloques {
		if reHashtag.MatchString(b.Texto()) {
			return true
		}
	}
	return false
}

// partirHashtags parte un tramo de texto en el texto de alrededor y sus hashtags.
func partirHashtags(t Tramo) []Tramo {
	if t.Enlace != "" || t.Imagen != nil || t.Salto || slices.Contains(t.Marcas, "code") {
		return []Tramo{t}
	}
	var out []Tramo
	resto, desde := t.Texto, 0
	for _, m := range reHashtag.FindAllStringSubmatchIndex(resto, -1) {
		// m[4]:m[5] es la palabra tras `#`; el `#` está justo antes.
		tag := etiquetaDe(resto[m[4]:m[5]])
		if tag == "" {
			continue
		}
		almohadilla := m[4] - 1
		if almohadilla > desde {
			antes := t
			antes.Texto = resto[desde:almohadilla]
			out = append(out, antes)
		}
		h := t
		h.Texto, h.Enlace = "#"+tag, hrefHashtag(tag)
		out = append(out, h)
		desde = m[4] + len(tag)
	}
	if desde == 0 {
		return []Tramo{t}
	}
	if desde < len(resto) {
		despues := t
		despues.Texto = resto[desde:]
		out = append(out, despues)
	}
	return out
}
