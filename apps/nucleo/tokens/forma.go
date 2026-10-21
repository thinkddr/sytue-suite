// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	_ "embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// temaCSS es la copia de `apps/frontend/packages/config/src/theme.css`, que es donde la
// web declara las MEDIDAS de la forma (radios, rellenos, pesos): su bloque «Tokens de
// forma». La deja scripts/sync-branding.sh y un test la compara con la fuente, como las
// otras dos hojas. Aquí no se copia ni un número: se lee el token.
//
//go:embed theme.css
var temaCSS string

// Medida es el valor de un token de forma tal como lo escribe la hoja: un número y su
// unidad ("px", "rem" o ninguna, que es un peso o una escala). Quien pinta la convierte
// a su píxel: el rem es del toolkit, no del núcleo.
type Medida struct {
	Valor  float64
	Unidad string
}

// formas son las medidas de cada forma, ya resueltas: [0] Marco, [1] Carril.
var formas = mustFormas(temaCSS, aparienciaCSS)

// MedidaDeForma devuelve el token de forma `nombre` —"radius-control", "sheet-gap",
// "nav-pad", "title-weight"…, sin su `--` ni su `sytue-`— en la forma que esté en vigor
// (ver ElegirForma). Un nombre que no existe entra en pánico, como Color.
func MedidaDeForma(nombre string) Medida { return MedidaEnForma(nombre, EsCarril()) }

// MedidaEnForma es MedidaDeForma para una forma concreta.
func MedidaEnForma(nombre string, carril bool) Medida {
	i := 0
	if carril {
		i = 1
	}
	m, ok := formas[i][nombre]
	if !ok {
		panic("tokens: no existe el token de forma " + nombre)
	}
	return m
}

var (
	reDeclCualquiera = regexp.MustCompile(`--([a-z0-9-]+)\s*:\s*([^;{}]+);`)
	reVarCualquiera  = regexp.MustCompile(`^var\(--([a-z0-9-]+)\)$`)
	reMedida         = regexp.MustCompile(`^(-?\d*\.?\d+)(px|rem)?$`)
)

// medidasDeForma son los tokens de forma que las nativas pintan: si la hoja deja de
// declarar uno, el binario no arranca.
var medidasDeForma = []string{
	"radius-control", "radius-nav", "radius-card", "radius-row", "radius-sheet",
	"sheet-gap", "nav-pad", "nav-indicator", "nav-active-weight", "rail-indicator",
	"row-grow", "row-inset", "row-bar",
	"title-weight", "subtitle-weight", "title-scale",
}

// La regla que pone los títulos de la hoja en Marco. No es un bloque de tokens sino una
// regla de clase de appearance.css, y es la única de la que se leen medidas: los
// títulos solo cambian DENTRO de la hoja, y en las nativas todo el contenido lo está.
const hojaDeMarco = ":root:not([data-forma='carril']) .sytue-sheet"

// mustFormas lee las medidas de las dos formas.
//
// Marco es la hoja sin atributo: lo que theme.css declara en sus bloques de primer nivel
// (`@theme` y `:root`), más lo que appearance.css cambia dentro de la hoja. Carril es
// eso mismo con `[data-forma='carril']` encima. Los bloques dentro de un `@media` de
// ancho no cuentan: que la hoja vaya a sangre en estrecho lo decide quien la coloca.
func mustFormas(tema, apariencia string) [2]map[string]Medida {
	base, carril := map[string]string{}, map[string]string{}
	for _, b := range bloquesDe(tema) {
		switch {
		case len(b.ruta) == 1 && (b.ruta[0] == ":root" || strings.HasPrefix(b.ruta[0], "@theme")):
			declarar(base, b.cuerpo)
		case len(b.ruta) == 1 && b.ruta[0] == "[data-forma='carril']":
			declarar(carril, b.cuerpo)
		}
	}
	marco := map[string]string{}
	for _, b := range bloquesDe(apariencia) {
		if b.ruta[len(b.ruta)-1] == hojaDeMarco && (len(b.ruta) == 1 || b.ruta[0] == "@media screen") {
			declarar(marco, b.cuerpo)
		}
	}
	var out [2]map[string]Medida
	for i, encima := range []map[string]string{marco, carril} {
		out[i] = map[string]Medida{}
		for nombre := range base {
			valor, ok := encima[nombre]
			if !ok {
				valor = base[nombre]
			}
			// Solo lo que es una medida: theme.css declara también colores, fuentes y
			// sombras, que no son de aquí.
			if m, ok := medida(valor, base); ok {
				out[i][strings.TrimPrefix(nombre, "sytue-")] = m
			}
		}
	}
	for _, nombre := range medidasDeForma {
		for i := range out {
			if _, ok := out[i][nombre]; !ok {
				panic(fmt.Sprintf("tokens: theme.css ya no declara el token de forma %s", nombre))
			}
		}
	}
	return out
}

// declarar apunta las custom properties de un bloque; la última gana, como en CSS.
func declarar(en map[string]string, cuerpo string) {
	for _, m := range reDeclCualquiera.FindAllStringSubmatch(cuerpo, -1) {
		en[m[1]] = strings.TrimSpace(m[2])
	}
}

// medida lee un valor de la hoja como número con unidad, siguiendo un `var()` si lo es.
func medida(valor string, base map[string]string) (Medida, bool) {
	for range 4 { // un var() que apunta a otro, como mucho unas pocas veces
		m := reVarCualquiera.FindStringSubmatch(valor)
		if m == nil {
			break
		}
		valor = base[m[1]]
	}
	m := reMedida.FindStringSubmatch(valor)
	if m == nil {
		return Medida{}, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	return Medida{Valor: n, Unidad: m[2]}, err == nil
}

// bloque es un bloque de declaraciones de una hoja: su selector, precedido de los
// `@media` (o lo que sea) que lo envuelven, y el texto que tiene DIRECTAMENTE dentro.
type bloque struct {
	ruta   []string
	cuerpo string
}

// bloquesDe recorre una hoja llave a llave. No es un parser de CSS: le basta con saber
// qué declaraciones cuelgan de qué selector, que es lo que la hoja de tokens necesita.
func bloquesDe(css string) []bloque {
	css = normalizar(css)
	var (
		out     []bloque
		ruta    []string
		cuerpos []*strings.Builder // lo que cada bloque abierto lleva directamente dentro
		desde   int
	)
	for i := 0; i < len(css); i++ {
		switch css[i] {
		case '{':
			// Hasta la llave va lo que quedaba del bloque de fuera y, tras su último
			// punto y coma, el selector del que se abre.
			antes, selector := "", css[desde:i]
			if j := strings.LastIndexByte(selector, ';'); j >= 0 {
				antes, selector = selector[:j+1], selector[j+1:]
			}
			if len(cuerpos) > 0 {
				cuerpos[len(cuerpos)-1].WriteString(antes)
			}
			ruta = append(ruta, reEspacios.ReplaceAllString(strings.TrimSpace(selector), " "))
			cuerpos = append(cuerpos, &strings.Builder{})
			desde = i + 1
		case '}':
			if len(ruta) > 0 {
				ultimo := len(ruta) - 1
				cuerpos[ultimo].WriteString(css[desde:i])
				out = append(out, bloque{ruta: append([]string{}, ruta...), cuerpo: cuerpos[ultimo].String()})
				ruta, cuerpos = ruta[:ultimo], cuerpos[:ultimo]
			}
			desde = i + 1
		}
	}
	return out
}
