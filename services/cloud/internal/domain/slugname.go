// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Nombres sugeridos para proyectos, en los cinco idiomas de producto
// ---------------------------------------------------------------------------
//
// El slug es el subdominio público de la app y es único GLOBALMENTE, así que quien
// crea un proyecto se inventa un nombre, choca con un 409 y no puede saber por qué
// —lo tiene otra organización— ni qué probar después. Sugerir un nombre libre
// convierte «crear proyecto» en aceptar lo que ya está escrito.
//
// Tres decisiones que no son de adorno:
//
//   - Cinco idiomas CON SU GRAMÁTICA (docs/CONVENTIONS.md §13.1), no una traducción
//     palabra a palabra del español. En español, francés e italiano el adjetivo va
//     detrás y concuerda en género; en inglés va delante y no concuerda; en alemán
//     va delante, hay tres géneros y el adjetivo antepuesto sin artículo se declina
//     en nominativo fuerte. Emparejar al azar produciría «duna sereno» o
//     «ruhig hafen», que se leen como una máquina a la primera pasada.
//   - Sin tildes ni eñes POR CONSTRUCCIÓN, no por transliteración: las palabras del
//     catálogo se eligieron ya sin acentos (ver slugname_catalog.go).
//   - El idioma se NEGOCIA (ver NegotiateLanguage) en vez de fijarse: el mismo
//     endpoint sirve al dashboard, al CLI y a un cliente de terceros.

// SlugLanguages son los idiomas de producto. El primero es el BASE: lo que se usa
// cuando la petición no pide ninguno o pide uno que no hablamos.
var SlugLanguages = []string{"en", "es", "fr", "de", "it"}

// BaseLanguage es el idioma base del producto (§13.1).
const BaseLanguage = "en"

// slugGender: el alemán obliga a tener tres. Las lenguas sin neutro no usan la
// tercera casilla.
type slugGender uint8

const (
	slugGenderMasculine slugGender = iota
	slugGenderFeminine
	slugGenderNeuter
)

// slugNoun es un sustantivo del catálogo con su género, que es quien manda sobre
// la forma del adjetivo.
type slugNoun struct {
	word   string
	gender slugGender
}

// slugAdjective guarda una forma por género, indexada por slugGender. Los
// invariables repiten la misma en las tres: es más simple que un caso especial y
// hace imposible olvidarse de uno.
type slugAdjective [3]string

// slugCatalog es el vocabulario de un idioma más su regla de orden.
type slugCatalog struct {
	nouns      []slugNoun
	adjectives []slugAdjective
	// adjectiveFirst: inglés y alemán ponen el adjetivo delante; español, francés
	// e italiano, detrás.
	adjectiveFirst bool
}

var slugCatalogs = map[string]*slugCatalog{
	"en": &slugCatalogEN,
	"es": &slugCatalogES,
	"fr": &slugCatalogFR,
	"de": &slugCatalogDE,
	"it": &slugCatalogIT,
}

// compose une sustantivo y adjetivo en el orden del idioma, con el adjetivo en la
// forma que pide el género del sustantivo.
func (c *slugCatalog) compose(n slugNoun, a slugAdjective) string {
	form := a[n.gender]
	if c.adjectiveFirst {
		return form + "-" + n.word
	}
	return n.word + "-" + form
}

// invariableAdjectives devuelve los adjetivos que valen para CUALQUIER género.
// Se deducen de que las tres formas coincidan, en vez de mantenerse en una lista
// aparte que se desincronizaría al añadir palabras.
//
// Sirven para las alternativas de un nombre que escribió el usuario: de su
// sustantivo no sabemos el género, y «mi-tienda-sereno» delata la máquina igual
// que «duna sereno». En alemán la lista sale VACÍA a propósito —el adjetivo
// antepuesto siempre se declina, así que sin conocer el género no hay forma
// correcta— y las alternativas alemanas se apoyan en el sufijo numérico y en
// nombres del catálogo, que sí están bien formados.
func (c *slugCatalog) invariableAdjectives() []string {
	var out []string
	for _, a := range c.adjectives {
		if a[0] == a[1] && a[1] == a[2] {
			out = append(out, a[0])
		}
	}
	return out
}

// catalogFor resuelve el catálogo de un idioma, con el base como red.
func catalogFor(lang string) *slugCatalog {
	if c, ok := slugCatalogs[lang]; ok {
		return c
	}
	return slugCatalogs[BaseLanguage]
}

// SuggestSlug devuelve un nombre sugerido en `lang`: sustantivo y adjetivo en el
// orden y la concordancia de esa lengua.
//
// `attempt` es el número de intento (0 = el primero). A partir del segundo se añade
// un número, porque el catálogo es finito y el slug es único en toda la plataforma:
// sin sufijo, con el catálogo agotado no habría forma de proponer nada. El número
// va al final y no en medio para que el nombre se siga leyendo igual.
//
// La aleatoriedad es de math/rand: esto es una sugerencia de nombre, no un secreto.
// Un nombre adivinable no da acceso a nada —el proyecto se crea autenticado y vive
// bajo la RLS de su tenant—, así que gastar entropía criptográfica aquí sería
// confundir «aleatorio» con «impredecible».
//
// nombre visible en la interfaz, no un secreto ni un identificador de sesión.
//
//nolint:gosec // G404 deliberado: ver el párrafo de arriba. Es una sugerencia de
func SuggestSlug(lang string, attempt int) string {
	c := catalogFor(lang)
	name := c.compose(
		c.nouns[rand.IntN(len(c.nouns))],
		c.adjectives[rand.IntN(len(c.adjectives))],
	)
	if attempt > 0 {
		name = fmt.Sprintf("%s-%d", name, rand.IntN(900)+100)
	}
	return name
}

// SlugSuggestionAttempts es cuántos nombres se prueban antes de rendirse. Con el
// sufijo numérico desde el segundo intento, agotarlos significa mala suerte
// extraordinaria; el tope existe para que un fallo de la comprobación no deje el
// bucle girando contra la base de datos.
const SlugSuggestionAttempts = 8

// SlugAlternatives propone hasta `n` nombres DERIVADOS del que escribió el usuario,
// para cuando el suyo está ocupado. No comprueba disponibilidad: eso lo hace la
// capa de servicio con una sola consulta para todos (ver service.CheckSlug).
//
// El orden es deliberado: primero el sufijo numérico —la variante que conserva
// intacto el nombre elegido—, luego el mismo nombre con un adjetivo invariable, y
// solo al final nombres del catálogo, que ya no tienen relación con lo que el
// usuario quería. Un sufijo numérico previo se recorta antes de añadir el nuevo
// para no acabar en «mi-tienda-2-2».
//
// Todo candidato pasa por ValidSlug: si el nombre del usuario es largo, las
// variantes que se salgan de los 40 caracteres se descartan solas y quedan las
// del catálogo, que siempre caben.
func SlugAlternatives(base, lang string, n int) []string {
	if n <= 0 {
		return nil
	}
	c := catalogFor(lang)
	stem := trimNumericSuffix(base)
	out := make([]string, 0, n)
	add := func(s string) {
		if len(out) < n && s != base && ValidSlug(s) && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}

	// Dos numéricas y no más: a partir de la tercera, «-4» ya no aporta nada que no
	// dijera «-3», y el hueco se aprovecha mejor con variantes que se lean distinto.
	for i := 2; i <= 3; i++ {
		add(stem + "-" + strconv.Itoa(i))
	}
	adjectives := c.invariableAdjectives()
	// G404 no aplica: esto baraja ADJETIVOS para proponer nombres libres de app
	// («mi-app-veloz»). No hay nada que adivinar —las sugerencias se enseñan— y
	// crypto/rand aquí solo añadiría coste a una lista de palabras.
	rand.Shuffle(len(adjectives), func(i, j int) { //nolint:gosec // G404: variedad estética, no aleatoriedad con valor de seguridad
		adjectives[i], adjectives[j] = adjectives[j], adjectives[i]
	})
	for _, a := range adjectives {
		if c.adjectiveFirst {
			add(a + "-" + stem)
		} else {
			add(stem + "-" + a)
		}
	}
	// Red final: nombres del catálogo. La primera vuelta los pide LIMPIOS (sin
	// sufijo numérico): con cientos de combinaciones por idioma el azar no se repite
	// en una docena de tiradas, y «flache-quelle» se lee mejor que
	// «flache-quelle-893». El sufijo entra solo si de verdad hiciera falta. Los
	// intentos se acotan porque `add` puede rechazar repetidos y esto no puede
	// quedarse girando.
	for attempt := 0; len(out) < n && attempt < 4*n; attempt++ {
		add(SuggestSlug(lang, attempt/n))
	}
	return out
}

// trimNumericSuffix quita un «-123» final si lo hay. Devuelve la entrada tal cual
// cuando no hay sufijo o cuando quitarlo dejaría el nombre vacío.
func trimNumericSuffix(s string) string {
	i := strings.LastIndexByte(s, '-')
	if i <= 0 || i == len(s)-1 {
		return s
	}
	for _, r := range s[i+1:] {
		if r < '0' || r > '9' {
			return s
		}
	}
	return s[:i]
}

// NegotiateLanguage elige el idioma de una petición: el parámetro explícito si lo
// hay y lo hablamos, si no la cabecera Accept-Language, y si no el base.
//
// Es una función PURA (entra texto, sale un idioma) para poder testear el sitio
// donde de verdad se equivoca uno: la `q` ausente vale 1 —no 0—, hay que ordenar
// por preferencia y no por orden de aparición, y una etiqueta regional cuyo idioma
// no hablamos («pt-BR») no puede cortar la negociación: se pasa a la siguiente
// candidata. Se acepta «es-419» o «ES_es» como español: lo que decide es la
// subetiqueta primaria.
func NegotiateLanguage(explicit, acceptLanguage string) string {
	if l, ok := supportedLanguage(explicit); ok {
		return l
	}

	type preference struct {
		tag  string
		q    float64
		rank int // posición original: desempata sin reordenar lo que el cliente ya ordenó
	}
	var prefs []preference
	for i, item := range strings.Split(acceptLanguage, ",") {
		parts := strings.Split(item, ";")
		tag := strings.TrimSpace(parts[0])
		if tag == "" {
			continue
		}
		q := 1.0 // Sin «q» explícita la preferencia es máxima (RFC 9110 §12.4.2).
		for _, p := range parts[1:] {
			p = strings.TrimSpace(p)
			if !strings.HasPrefix(p, "q=") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimPrefix(p, "q="), 64)
			if err != nil {
				q = 0 // Una «q» ilegible no puede colarse como preferencia máxima.
				break
			}
			q = parsed
		}
		if q <= 0 { // q=0 es un rechazo explícito de ese idioma.
			continue
		}
		prefs = append(prefs, preference{tag: tag, q: q, rank: i})
	}
	slices.SortStableFunc(prefs, func(a, b preference) int {
		if a.q != b.q {
			if a.q > b.q {
				return -1
			}
			return 1
		}
		return a.rank - b.rank
	})

	for _, p := range prefs {
		// «*» significa «lo que tengas»: el base, que es lo que mejor mantenemos.
		if p.tag == "*" {
			return BaseLanguage
		}
		if l, ok := supportedLanguage(p.tag); ok {
			return l
		}
	}
	return BaseLanguage
}

// supportedLanguage normaliza una etiqueta BCP-47 a un idioma de producto: se queda
// con la subetiqueta primaria («fr-CA» -> «fr») y comprueba que la hablemos.
func supportedLanguage(tag string) (string, bool) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		tag = tag[:i]
	}
	if tag == "" || !slices.Contains(SlugLanguages, tag) {
		return "", false
	}
	return tag, true
}
