// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// BaseLanguage es el idioma base del producto (CONVENTIONS §13.1): las cadenas
// nacen en inglés y los otros cuatro idiomas son traducciones de ese original.
const BaseLanguage = "en"

// supportedLanguages son los cinco idiomas de producto, con la base primero.
// Lista fija a propósito: §13.1 es normativa, no configuración por servicio.
var supportedLanguages = []string{BaseLanguage, "es", "fr", "de", "it"}

// Languages devuelve los idiomas de producto soportados (copia; base primero).
func Languages() []string { return slices.Clone(supportedLanguages) }

// NegotiateLanguage elige el idioma de respuesta a partir de una cabecera
// Accept-Language. Función pura y sin estado: se decide solo por la cabecera.
//
// Reglas (RFC 9110 §12.5.4), con las trampas habituales resueltas:
//   - `q` ausente vale 1; `q=0` significa «no aceptable» y descarta el rango.
//   - se ordena por `q` descendente, NO por orden de aparición; a igual `q`
//     gana el que aparece antes (orden estable).
//   - se casa por subetiqueta primaria: `fr-CA` casa con `fr`.
//   - un idioma que no soportamos (`pt-BR`) no bloquea: se pasa al siguiente
//     candidato por `q`.
//   - el comodín `*` significa «cualquiera»: devuelve la base.
//   - cabecera vacía, malformada o sin ningún candidato soportado → base.
//
// Solo se compara la subetiqueta primaria porque el producto no distingue
// variantes regionales (no hay es-MX frente a es-ES). Si algún día las hubiera,
// hay que ampliar el matching, no la negociación.
func NegotiateLanguage(header string) string {
	type candidate struct {
		lang string
		q    float64
	}
	var candidates []candidate
	for _, part := range strings.Split(header, ",") {
		lang, q, ok := parseLanguageRange(part)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate{lang: lang, q: q})
	}
	// Estable: a igual q se respeta el orden de la cabecera.
	slices.SortStableFunc(candidates, func(a, b candidate) int {
		switch {
		case a.q > b.q:
			return -1
		case a.q < b.q:
			return 1
		default:
			return 0
		}
	})
	for _, c := range candidates {
		if c.lang == "*" {
			return BaseLanguage
		}
		if slices.Contains(supportedLanguages, c.lang) {
			return c.lang
		}
	}
	return BaseLanguage
}

// LanguageFromRequest negocia el idioma de la petición. Es lo que deben usar los
// handlers que GENERAN texto (correos, sugeridor de nombres, exportaciones): el
// mensaje de error de la API no se traduce en el servidor, ver Error.
func LanguageFromRequest(r *http.Request) string {
	if r == nil {
		return BaseLanguage
	}
	return NegotiateLanguage(r.Header.Get("Accept-Language"))
}

// parseLanguageRange analiza un elemento de Accept-Language ("fr-CA;q=0.8").
// Devuelve la subetiqueta primaria en minúsculas (o "*") y su calidad. ok=false
// descarta el elemento: sintaxis inválida o q=0 («no aceptable»).
func parseLanguageRange(part string) (lang string, q float64, ok bool) {
	fields := strings.Split(part, ";")
	tag := strings.ToLower(strings.TrimSpace(fields[0]))
	if tag != "*" {
		primary, _, _ := strings.Cut(tag, "-")
		if !validPrimarySubtag(primary) {
			return "", 0, false
		}
		tag = primary
	}

	q = 1.0 // q ausente vale 1.
	for _, param := range fields[1:] {
		name, value, found := strings.Cut(param, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "q") {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || v < 0 || v > 1 {
			return "", 0, false // q basura: elemento inválido, se ignora.
		}
		q = v
	}
	if q == 0 {
		return "", 0, false
	}
	return tag, q, true
}

// validPrimarySubtag comprueba la subetiqueta primaria de RFC 5646: de 1 a 8
// letras ASCII. Todo lo demás es basura y se descarta.
func validPrimarySubtag(s string) bool {
	if s == "" || len(s) > 8 {
		return false
	}
	for _, c := range s {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}
