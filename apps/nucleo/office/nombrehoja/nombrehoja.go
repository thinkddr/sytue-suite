// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package nombrehoja dice cuándo dos hojas de un libro se llaman igual: sin distinguir
// mayúsculas, como Excel y LibreOffice, que no dejan tener «Ventas» y «VENTAS» en el
// mismo libro. Con las dos, una referencia `VENTAS!A1` confunde a quien la lee y los
// desplazamientos de la web (`mismaHoja` en `refshift.ts`) ya comparan sin mayúsculas.
//
// Es la misma regla para el servicio docs (que rechaza el guardado que crea una gemela),
// para Calcus nativo (que no deja crearla) y para el importador (que renombra la
// segunda). Vive en el núcleo porque los tres lo importan; la web la repite en
// `packages/office/src/editor/sheets/pestana.ts`, nunca más laxa que esta.
package nombrehoja

import (
	"strconv"
	"strings"
	"unicode"
)

// Gemelos dice si dos nombres de hoja son el mismo salvo mayúsculas («Ventas» y
// «VENTAS»). Un nombre es gemelo de sí mismo.
func Gemelos(a, b string) bool { return strings.EqualFold(a, b) }

// clave es el nombre plegado: cada letra cambiada por la menor de su órbita de
// mayúsculas (`unicode.SimpleFold`). `strings.EqualFold` compara letra a letra por esas
// órbitas, así que Gemelos(a, b) es clave(a) == clave(b), y la clave sirve de índice.
func clave(s string) string {
	return strings.Map(func(r rune) rune {
		m := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			m = min(m, f)
		}
		return m
	}, s)
}

// Gemela es la hoja de `nombres`, distinta de `excepto`, que se llama como `nombre` sin
// distinguir mayúsculas; -1 si no hay ninguna. Con `excepto` a -1 las mira todas.
func Gemela(nombres []string, nombre string, excepto int) int {
	for i, n := range nombres {
		if i != excepto && Gemelos(n, nombre) {
			return i
		}
	}
	return -1
}

// HayGemelas dice si dos hojas de `nombres` se llaman igual sin distinguir mayúsculas.
func HayGemelas(nombres []string) bool {
	vistas := make(map[string]bool, len(nombres))
	for _, n := range nombres {
		k := clave(n)
		if vistas[k] {
			return true
		}
		vistas[k] = true
	}
	return false
}

// Libre es el primer nombre de la serie `nombrar(desde)`, `nombrar(desde+1)`… que no es
// gemelo de ninguna hoja de `nombres`: «Hoja 3» si está libre y si no «Hoja 4»; «Datos
// (2)», «Datos (3)»… `nombrar` da un nombre distinto para cada número.
func Libre(nombres []string, nombrar func(n int) string, desde int) string {
	ocupados := make(map[string]bool, len(nombres))
	for _, n := range nombres {
		ocupados[clave(n)] = true
	}
	// Cada hoja ocupa a lo sumo un nombre de la serie: con len+1 intentos hay uno libre.
	n := desde
	for ; n < desde+len(nombres); n++ {
		if s := nombrar(n); !ocupados[clave(s)] {
			return s
		}
	}
	return nombrar(n)
}

// GemelaNueva es la regla del servidor: el primer nombre de `nuevos` que un guardado trae
// gemelo de otra hoja sin que el libro lo tuviera ya así en `previos` —una hoja creada, o
// renombrada, con el nombre de otra salvo mayúsculas—, o "" si no hay ninguno.
//
// Las gemelas que el libro ya tenía (un importado de antes de que el importador las
// reparara, un libro de antes de esta regla) siguen valiendo: cada nombre exacto puede
// salir tantas veces como salía, y el libro se sigue pudiendo editar sin renombrarlas.
func GemelaNueva(previos, nuevos []string) string {
	antes := make(map[string]int, len(previos))
	for _, n := range previos {
		antes[n]++
	}
	ahora := make(map[string]int, len(nuevos))
	grupos := make(map[string]int, len(nuevos))
	for _, n := range nuevos {
		ahora[n]++
		grupos[clave(n)]++
	}
	for _, n := range nuevos {
		if grupos[clave(n)] > 1 && ahora[n] > antes[n] {
			return n
		}
	}
	return ""
}

// Desgemelar devuelve los nombres con cada gemela de una hoja anterior renombrada como
// la renombra Excel al reparar un libro: «VENTAS (2)», «VENTAS (3)»…, sin chocar con
// ninguna otra hoja. La primera de cada grupo se queda como está. Sin gemelas, devuelve
// `nombres` tal cual.
func Desgemelar(nombres []string) []string {
	if !HayGemelas(nombres) {
		return nombres
	}
	out := make([]string, len(nombres))
	copy(out, nombres)
	// Los nombres de todas las hojas quedan reservados: «Ventas (2)» no se le da a una
	// gemela si otra hoja, más adelante, ya se llama así.
	tomados := make(map[string]bool, len(nombres))
	for _, n := range nombres {
		tomados[clave(n)] = true
	}
	primeras := make(map[string]bool, len(nombres))
	for i, n := range nombres {
		k := clave(n)
		if !primeras[k] {
			primeras[k] = true
			continue
		}
		for s := 2; ; s++ {
			c := n + " (" + strconv.Itoa(s) + ")"
			if kc := clave(c); !tomados[kc] {
				out[i], tomados[kc] = c, true
				break
			}
		}
	}
	return out
}
