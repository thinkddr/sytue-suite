// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package i18n traduce las pantallas del IdP a los cinco idiomas de producto
// (docs/CONVENTIONS.md §13.1): inglés base más español, francés, alemán e
// italiano.
//
// Es un catálogo en Go y no una librería de i18n a propósito. Sytue ID se sirve
// como binario Go sin build de frontend, así que no puede consumir el i18n de
// Astro; y las alternativas (go-i18n, x/text/message) traen ficheros de
// traducción que cargar en runtime, plurales CLDR y una dependencia más en la
// cadena de suministro, para un conjunto de cadenas que cabe en un fichero. Con
// un mapa, el compilador ve las claves y un test comprueba que no falta ningún
// idioma.
//
// El texto EN INGLÉS es el original: las cadenas nacen ahí y el resto son
// traducciones, como en el resto de la plataforma.
package i18n

import (
	"net/http"
	"strings"
)

// Locales son los idiomas soportados, en el orden canónico de presentación
// (el base primero, el resto alfabético por código).
var Locales = []string{"en", "bg", "cs", "da", "de", "el", "es", "et", "fi", "fr", "ga", "hr", "hu", "is", "it", "lt", "lv", "mt", "nb", "nl", "pl", "pt", "ro", "sk", "sl", "sv"}

// Default es el idioma al que se cae cuando no se puede determinar otro.
const Default = "en"

// CookieName es la cookie de idioma compartida por la suite. Misma que lee
// apps/frontend/packages/i18n: si el usuario elige idioma en «Mi cuenta», el
// resto de superficies lo respetan sin volver a preguntárselo.
const CookieName = "sytue_locale"

// Normalize reduce lo que llegue a uno de los idiomas soportados. Acepta
// etiquetas BCP-47 completas ("es-ES", "en-GB") quedándose con la subetiqueta
// de idioma, que es lo que mandan los navegadores.
func Normalize(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if base, _, ok := strings.Cut(v, "-"); ok {
		v = base
	}
	for _, l := range Locales {
		if v == l {
			return v
		}
	}
	return Default
}

// From decide en qué idioma servir una petición, con la MISMA precedencia que
// el resto de la plataforma: elección explícita del usuario (cookie) por encima
// de la preferencia del navegador (Accept-Language), y el inglés como último
// recurso.
//
// La cookie manda porque es una decisión consciente: quien la fijó quiere ese
// idioma aunque su navegador esté en otro, que es justo el caso de quien usa un
// ordenador prestado o un sistema operativo en un idioma que no domina.
func From(r *http.Request) string {
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		if l := Normalize(c.Value); l != Default || strings.HasPrefix(strings.ToLower(c.Value), "en") {
			return l
		}
	}
	return fromAcceptLanguage(r.Header.Get("Accept-Language"))
}

// fromAcceptLanguage escoge el primer idioma soportado de la cabecera, honrando
// los factores de calidad. No implementa RFC 4647 entero: ordena por q y coge el
// primero que conocemos, que es todo lo que hace falta con cinco idiomas.
func fromAcceptLanguage(header string) string {
	if header == "" {
		return Default
	}
	mejor, mejorQ := Default, -1.0
	for _, parte := range strings.Split(header, ",") {
		etiqueta, q := parte, 1.0
		if i := strings.IndexByte(parte, ';'); i >= 0 {
			etiqueta = parte[:i]
			if v, ok := parseQ(parte[i+1:]); ok {
				q = v
			}
		}
		etiqueta = strings.TrimSpace(etiqueta)
		if etiqueta == "" || etiqueta == "*" {
			continue
		}
		// Normalize devuelve "en" tanto para el inglés como para lo desconocido, así
		// que se comprueba que la etiqueta sea realmente de un idioma soportado
		// antes de puntuarla: si no, un "Accept-Language: pt" ganaría como inglés.
		l := Normalize(etiqueta)
		if !soportado(etiqueta) {
			continue
		}
		if q > mejorQ {
			mejor, mejorQ = l, q
		}
	}
	return mejor
}

// soportado dice si la etiqueta corresponde a un idioma del catálogo (mirando
// solo su subetiqueta de idioma).
func soportado(etiqueta string) bool {
	base := strings.ToLower(strings.TrimSpace(etiqueta))
	if b, _, ok := strings.Cut(base, "-"); ok {
		base = b
	}
	for _, l := range Locales {
		if base == l {
			return true
		}
	}
	return false
}

// parseQ lee el factor de calidad de un parámetro ";q=0.8".
func parseQ(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "q=") && !strings.HasPrefix(s, "Q=") {
		return 0, false
	}
	s = strings.TrimSpace(s[2:])
	// Los valores válidos son 0, 1 o 0.xxx / 1.000 (RFC 9110 §12.4.2). Se parsea a
	// mano para no arrastrar strconv por tres formas posibles.
	switch {
	case s == "0":
		return 0, true
	case s == "1":
		return 1, true
	case strings.HasPrefix(s, "0."):
		return fraccion(s[2:]), true
	case strings.HasPrefix(s, "1."):
		return 1, true
	}
	return 0, false
}

// fraccion convierte los decimales de un q ("8" -> 0.8, "75" -> 0.75).
func fraccion(d string) float64 {
	valor, escala := 0.0, 1.0
	for _, r := range d {
		if r < '0' || r > '9' {
			break
		}
		escala *= 10
		valor = valor*10 + float64(r-'0')
	}
	if escala == 1 {
		return 0
	}
	return valor / escala
}

// T traduce una clave al idioma dado.
//
// Si falta la traducción cae al inglés, y si tampoco está devuelve la clave: una
// pantalla con una clave visible se detecta a simple vista, mientras que una
// cadena vacía pasa por hueco de maquetación y puede vivir años.
func T(locale, key string) string {
	if m, ok := catalogo[key]; ok {
		if s := m[locale]; s != "" {
			return s
		}
		if s := m[Default]; s != "" {
			return s
		}
	}
	return key
}

// Name da el nombre de cada idioma EN SU PROPIO IDIOMA. No se traduce nunca: es
// lo que permite volver al suyo a quien no entiende el que está viendo.
var Name = map[string]string{
	"en": "English",
	"bg": "Български",
	"cs": "Čeština",
	"da": "Dansk",
	"de": "Deutsch",
	"el": "Ελληνικά",
	"es": "Español",
	"et": "Eesti",
	"fi": "Suomi",
	"fr": "Français",
	"ga": "Gaeilge",
	"hr": "Hrvatski",
	"hu": "Magyar",
	"is": "Íslenska",
	"it": "Italiano",
	"lt": "Lietuvių",
	"lv": "Latviešu",
	"mt": "Malti",
	"nb": "Norsk bokmål",
	"nl": "Nederlands",
	"pl": "Polski",
	"pt": "Português",
	"ro": "Română",
	"sk": "Slovenčina",
	"sl": "Slovenščina",
	"sv": "Svenska",
}
