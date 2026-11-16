// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las cabeceras A–Z de la lista (design.md §2.4: «List A–Z con Avatar»). Es la gemela de
// `gruposPorInicial` en apps/frontend/apps/contacts/src/client/iniciales.ts y las dos
// pasan los mismos vectores (testdata/iniciales.json).
//
// La inicial NO es «la primera letra sin tilde»: qué es una letra propia lo decide la
// colación del idioma, la misma que ordena la lista. En español la Ñ tiene cabecera y
// la Á va con la A; en sueco y finés la Å, la Ä y la Ö van al final, cada una con la
// suya; en alemán la Ä va con la A. Una tabla a mano se habría quedado corta con 26
// idiomas: se pregunta al cotejador si la letra y su base son la MISMA a nivel primario.

// SinLetra es la cabecera de lo que no empieza por letra (cifras, signos, vacío). Va
// siempre al final, como en Apple y Google.
const SinLetra = "#"

// GrupoInicial es una cabecera y los índices (en la lista de entrada) que van bajo ella.
type GrupoInicial struct {
	Inicial string
	Indices []int
}

// cotejador es el `Intl.Collator(intlTag(locale))` de la web. El bokmål va con la
// etiqueta del nynorsk: x/text no aplica la tailoring noruega a «nb» (ordena Å con la A,
// como el inglés) y el ICU de los navegadores sí. Comparten alfabeto (… Z Æ Ø Å).
func cotejador(l i18n.Locale, opciones ...collate.Option) *collate.Collator {
	tag := l.Tag()
	if l == "nb" {
		tag = language.MustParse("nn-NO")
	}
	return collate.New(tag, opciones...)
}

// GruposPorInicial agrupa los nombres por su inicial en el orden en que aparecen (la
// lista ya llega ordenada por la colación del idioma), con «#» al final. Un grupo que
// la colación dejara partido se junta en el sitio de su primera aparición.
func GruposPorInicial(nombres []string, l i18n.Locale) []GrupoInicial {
	base := cotejador(l, collate.Loose)
	cache := map[rune]string{}
	var grupos []GrupoInicial
	donde := map[string]int{}
	var sinLetra []int
	for i, n := range nombres {
		ini := inicialCon(n, base, cache)
		if ini == SinLetra {
			sinLetra = append(sinLetra, i)
			continue
		}
		g, ok := donde[ini]
		if !ok {
			g = len(grupos)
			donde[ini] = g
			grupos = append(grupos, GrupoInicial{Inicial: ini})
		}
		grupos[g].Indices = append(grupos[g].Indices, i)
	}
	if len(sinLetra) > 0 {
		grupos = append(grupos, GrupoInicial{Inicial: SinLetra, Indices: sinLetra})
	}
	return grupos
}

// inicialCon es la cabecera de un nombre. `cache` va por primera letra: en una libreta
// de miles, las iniciales distintas son unas decenas.
func inicialCon(nombre string, base *collate.Collator, cache map[rune]string) string {
	s := norm.NFC.String(strings.TrimSpace(nombre))
	r, _ := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError || !unicode.IsLetter(r) {
		return SinLetra
	}
	if ini, ok := cache[r]; ok {
		return ini
	}
	ini := inicialDeLetra(r, base)
	cache[r] = ini
	return ini
}

// inicialDeLetra pasa la letra a mayúscula y la reduce a su base si la colación dice
// que es la misma letra: Á → A en español, pero Ñ se queda. Las que no se descomponen
// (Ø, Ł) se prueban contra la A–Z latina: en inglés la Ø es una O; en danés, no.
func inicialDeLetra(r rune, base *collate.Collator) string {
	u := unicode.ToUpper(r)
	letra := string(u)
	if b, _ := utf8.DecodeRuneInString(norm.NFD.String(letra)); b != u && base.CompareString(letra, string(b)) == 0 {
		return string(b)
	}
	if (u < 'A' || u > 'Z') && unicode.Is(unicode.Latin, u) {
		for x := 'A'; x <= 'Z'; x++ {
			if base.CompareString(letra, string(x)) == 0 {
				return string(x)
			}
		}
	}
	return letra
}
