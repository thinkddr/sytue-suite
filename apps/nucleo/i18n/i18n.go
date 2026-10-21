// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package i18n es @sytue/i18n en Go: los mismos 26 catálogos, la misma negociación de
// idioma, el mismo plural por categorías CLDR y la misma interpolación.
//
// Los catálogos de locales/ son una copia GENERADA de
// apps/frontend/packages/i18n/src/messages (`go generate`); no se editan a mano.
package i18n

//go:generate bun ../scripts/gen-i18n.ts

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var comunes embed.FS

// Common es el catálogo común de la suite (el de @sytue/i18n), listo para New.
func Common() fs.FS {
	sub, err := fs.Sub(comunes, "locales")
	if err != nil {
		panic(err) // el directorio está embebido: no puede faltar
	}
	return sub
}

// Locale es uno de los 26 idiomas de la suite: los 24 oficiales de la UE, noruego e
// islandés.
type Locale string

// Default es el idioma de respaldo y la fuente de verdad de las cadenas.
const Default Locale = "en"

// Locales conserva el orden de `LOCALES` en locale.ts: es el del selector de idioma.
var Locales = []Locale{
	"en", "bg", "cs", "da", "de", "el", "es", "et", "fi", "fr", "ga", "hr", "hu",
	"is", "it", "lt", "lv", "mt", "nb", "nl", "pl", "pt", "ro", "sk", "sl", "sv",
}

// Names son los endónimos del selector (`LOCALE_NAMES`).
var Names = map[Locale]string{
	"en": "English", "bg": "Български", "cs": "Čeština", "da": "Dansk", "de": "Deutsch",
	"el": "Ελληνικά", "es": "Español", "et": "Eesti", "fi": "Suomi", "fr": "Français",
	"ga": "Gaeilge", "hr": "Hrvatski", "hu": "Magyar", "is": "Íslenska", "it": "Italiano",
	"lt": "Lietuvių", "lv": "Latviešu", "mt": "Malti", "nb": "Norsk bokmål",
	"nl": "Nederlands", "pl": "Polski", "pt": "Português", "ro": "Română",
	"sk": "Slovenčina", "sl": "Slovenščina", "sv": "Svenska",
}

// region es la de `INTL_TAG`: el inglés de la suite es el británico y el portugués el
// europeo, y de ahí salen separadores y reglas de plural.
var region = map[Locale]string{
	"en": "GB", "bg": "BG", "cs": "CZ", "da": "DK", "de": "DE", "el": "GR", "es": "ES",
	"et": "EE", "fi": "FI", "fr": "FR", "ga": "IE", "hr": "HR", "hu": "HU", "is": "IS",
	"it": "IT", "lt": "LT", "lv": "LV", "mt": "MT", "nb": "NO", "nl": "NL", "pl": "PL",
	"pt": "PT", "ro": "RO", "sk": "SK", "sl": "SI", "sv": "SE",
}

// Tag es la etiqueta BCP-47 con región ("es-ES"), la que la web le pasa a Intl.
func (l Locale) Tag() language.Tag { return language.MustParse(string(l) + "-" + region[l]) }

// IsLocale dice si s es uno de los 26.
func IsLocale(s string) bool { return slices.Contains(Locales, Locale(s)) }

// FromTag reduce una etiqueta del sistema ("es-ES", "pt_BR.UTF-8") a un idioma de la
// suite, por su subetiqueta primaria, como `documentLocale`.
func FromTag(tag string) (Locale, bool) {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
	base, _, _ = strings.Cut(base, "_")
	base, _, _ = strings.Cut(base, ".")
	return Locale(base), IsLocale(base)
}

var reQ = regexp.MustCompile(`(?i)^\s*q\s*=\s*(.*?)\s*$`)

// ParseAcceptLanguage elige el primer idioma servible de una cabecera Accept-Language
// (o de `navigator.languages` unido por comas): por calidad y, a igualdad, por orden.
func ParseAcceptLanguage(cabecera string) (Locale, bool) {
	type candidato struct {
		base    string
		calidad float64
	}
	var candidatos []candidato
	for _, parte := range strings.Split(cabecera, ",") {
		trozos := strings.Split(strings.TrimSpace(parte), ";")
		calidad := 1.0
		for _, p := range trozos[1:] {
			if m := reQ.FindStringSubmatch(p); m != nil {
				q, err := strconv.ParseFloat(m[1], 64)
				if err != nil || math.IsNaN(q) || math.IsInf(q, 0) {
					q = 0
				}
				calidad = q
				break
			}
		}
		base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(trozos[0])), "-")
		if base = strings.TrimSpace(base); calidad > 0 && base != "" {
			candidatos = append(candidatos, candidato{base, calidad})
		}
	}
	// Estable: a igual calidad manda el orden de la cabecera.
	sort.SliceStable(candidatos, func(i, j int) bool { return candidatos[i].calidad > candidatos[j].calidad })
	for _, c := range candidatos {
		if IsLocale(c.base) {
			return Locale(c.base), true
		}
	}
	return "", false
}

// Negotiate es `negotiateLocale`: la preferencia guardada manda sobre lo que pida el
// sistema, y sin ninguna de las dos, inglés. `guardada` es lo que en la web vive en la
// cookie `sytue_locale` y aquí en las preferencias de la app.
func Negotiate(aceptados, guardada string) Locale {
	if IsLocale(guardada) {
		return Locale(guardada)
	}
	if l, ok := ParseAcceptLanguage(aceptados); ok {
		return l
	}
	return Default
}

// Vars son las variables de una cadena. Los números se formatean con el idioma; lo
// demás entra tal cual.
type Vars map[string]any

// entrada es un valor de catálogo: una cadena, o las formas de plural por categoría
// CLDR (zero, one, two, few, many, other).
type entrada struct {
	texto  string
	formas map[string]string
}

func (e *entrada) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &e.texto)
	}
	return json.Unmarshal(b, &e.formas)
}

// Translator traduce a UN idioma. Es inmutable: se crea otro al cambiar de idioma.
type Translator struct {
	locale             Locale
	mensajes, respaldo map[string]entrada
}

// New carga el idioma pedido —y el inglés, de respaldo— de cada catálogo: el común
// (Common) y los de la app, que son directorios con un `<idioma>.json` por idioma. A
// un catálogo le puede faltar el idioma (cae al inglés, como `createAppTranslator`),
// pero no el inglés.
func New(l Locale, catalogos ...fs.FS) (*Translator, error) {
	if !IsLocale(string(l)) {
		return nil, fmt.Errorf("i18n: %q no es un idioma de la suite", l)
	}
	t := &Translator{locale: l, mensajes: map[string]entrada{}, respaldo: map[string]entrada{}}
	for _, c := range catalogos {
		if err := cargar(c, Default, t.respaldo); err != nil {
			return nil, err
		}
		if err := cargar(c, l, t.mensajes); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return t, nil
}

func cargar(c fs.FS, l Locale, en map[string]entrada) error {
	datos, err := fs.ReadFile(c, string(l)+".json")
	if err != nil {
		return fmt.Errorf("i18n: %w", err)
	}
	var m map[string]entrada
	if err := json.Unmarshal(datos, &m); err != nil {
		return fmt.Errorf("i18n: %s.json: %w", l, err)
	}
	for k, v := range m {
		en[k] = v
	}
	return nil
}

// Locale es el idioma de este traductor.
func (t *Translator) Locale() Locale { return t.locale }

// T traduce. Una clave que no está en el idioma cae al inglés, y si tampoco, se
// devuelve la propia clave: se ve en pantalla y se arregla, en vez de un hueco.
func (t *Translator) T(clave string, vars Vars) string {
	e, ok := t.mensajes[clave]
	if !ok {
		if e, ok = t.respaldo[clave]; !ok {
			return clave
		}
	}
	plantilla := e.texto
	if e.formas != nil {
		plantilla = t.plural(e.formas, vars)
	}
	return t.interpolar(plantilla, vars)
}

func (t *Translator) plural(formas map[string]string, vars Vars) string {
	categoria := Category(t.locale, aNumero(vars["count"]))
	if f, ok := formas[categoria]; ok {
		return f
	}
	// Un catálogo sin `two` (maltés) usa la de `few`, como translate.ts.
	if f, ok := formas["few"]; ok && categoria == "two" {
		return f
	}
	return formas["other"]
}

var rePlaceholder = regexp.MustCompile(`\{(\w+)\}`)

func (t *Translator) interpolar(plantilla string, vars Vars) string {
	if vars == nil {
		return plantilla
	}
	return rePlaceholder.ReplaceAllStringFunc(plantilla, func(m string) string {
		switch v := vars[m[1:len(m)-1]].(type) {
		case nil:
			return m
		case string:
			return v
		case fmt.Stringer:
			return v.String()
		default:
			return FormatNumber(t.locale, aNumero(v))
		}
	})
}

// aNumero es el `Number(x)` de JavaScript para lo que llega en unas Vars; lo que no es
// un número finito cuenta como 0, igual que en selectPlural.
func aNumero(v any) float64 {
	var n float64
	switch x := v.(type) {
	case int:
		n = float64(x)
	case int32:
		n = float64(x)
	case int64:
		n = float64(x)
	case uint:
		n = float64(x)
	case uint32:
		n = float64(x)
	case uint64:
		n = float64(x)
	case float32:
		n = float64(x)
	case float64:
		n = x
	case string:
		n, _ = strconv.ParseFloat(strings.TrimSpace(x), 64)
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	return n
}

var categorias = map[plural.Form]string{
	plural.Zero: "zero", plural.One: "one", plural.Two: "two",
	plural.Few: "few", plural.Many: "many", plural.Other: "other",
}

// Category es `new Intl.PluralRules(tag).select(n)`.
func Category(l Locale, n float64) string {
	// Los operandos de CLDR salen de cómo se ESCRIBE el número: Intl lo escribe con
	// hasta tres decimales y sin ceros de relleno.
	entero, decimales, _ := strings.Cut(strconv.FormatFloat(math.Abs(n), 'f', -1, 64), ".")
	if len(decimales) > 3 {
		decimales = decimales[:3]
	}
	decimales = strings.TrimRight(decimales, "0")
	i, _ := strconv.ParseInt(entero, 10, 64)
	f, _ := strconv.Atoi(decimales)
	sinDecimales := decimales == ""
	// ponytail: x/text lleva las reglas de CLDR 32 y los navegadores van por la 44+.
	// De los 26 idiomas cambiaron tres cosas, y el test contra Intl es quien lo dice;
	// el día que x/text se ponga al día, estos casos sobran.
	switch {
	case l == "mt":
		return maltes(i, sinDecimales)
	case l == "is" && !sinDecimales: // con decimales ya no es siempre `one`
		if t := int64(f); t%10 == 1 && t%100 != 11 {
			return "one"
		}
		return "other"
	case sinDecimales && i != 0 && i%1_000_000 == 0 && (l == "es" || l == "fr" || l == "it" || l == "pt"):
		return "many" // «1 000 000 de resultados»
	}
	const tope = 1_000_000_000 // MatchPlural trabaja con int; las reglas miran como mucho i % 1e6
	forma := plural.Cardinal.MatchPlural(l.Tag(), int(i%tope), len(decimales), len(decimales), f, f)
	return categorias[forma]
}

// maltes son las reglas de plural del maltés en CLDR 44: ganó `two`, y `few` dejó de
// incluir el 2 y el 102.
func maltes(i int64, sinDecimales bool) string {
	resto := i % 100
	switch {
	case !sinDecimales:
		return "other"
	case i == 1:
		return "one"
	case i == 2:
		return "two"
	case i == 0 || (resto >= 3 && resto <= 10):
		return "few"
	case resto >= 11 && resto <= 19:
		return "many"
	}
	return "other"
}
