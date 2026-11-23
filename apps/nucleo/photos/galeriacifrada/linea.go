// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package galeriacifrada

import (
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// # La línea de tiempo mezclada
//
// Las fotos del servidor llegan por páginas, de la más reciente a la más antigua; las
// cifradas están todas en el dispositivo. Para no pintar una cifrada de 2019 antes de
// que hayan llegado las de 2024 del servidor, una cifrada solo entra cuando la línea
// del servidor ya ha llegado hasta su fecha (o no tiene más páginas). Es la misma regla
// que la web (`apps/frontend/apps/photos/src/lib/lineaMezclada.ts`).
//
// Con una búsqueda el servidor ordena por lo que encuentra: las cifradas que coinciden
// (ya filtradas en el dispositivo) van delante, por fecha, y luego las suyas.

// OpcionesLinea son las de [MezclarLinea].
type OpcionesLinea struct {
	// Fin: la línea del servidor ya no tiene más páginas.
	Fin bool
	// Anyo: con un año elegido, solo lo de ese año y anteriores. 0, sin año.
	Anyo int
	// Busqueda: hay una búsqueda; las cifradas (ya filtradas) van delante.
	Busqueda bool
}

// ElementoLinea es una foto de la línea mezclada: del servidor o cifrada.
type ElementoLinea[S any] struct {
	Servidor S
	Cifrada  Foto
	// EsCifrada dice cuál de los dos campos vale.
	EsCifrada bool
	Tomada    time.Time
}

// MezclarLinea junta por fecha las fotos del servidor (`servidor`, ya ordenadas de la
// más reciente a la más antigua, con su fecha en `tomada`) y las cifradas (`cifradas`,
// en el mismo orden, como las da [Galeria.Leer]). A igual fecha, primero la del
// servidor, como la web.
func MezclarLinea[S any](servidor []S, tomada func(S) time.Time, cifradas []Foto, o OpcionesLinea) []ElementoLinea[S] {
	s := make([]ElementoLinea[S], len(servidor))
	for i, f := range servidor {
		s[i] = ElementoLinea[S]{Servidor: f, Tomada: tomada(f)}
	}
	var tope time.Time // cero: sin tope
	if o.Anyo != 0 {
		tope = time.Date(o.Anyo+1, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	// desde: la fecha más antigua que ya ha llegado del servidor. Sin fin y sin
	// ninguna foto del servidor todavía, no entra ninguna cifrada.
	ninguna := !o.Fin && len(servidor) == 0
	var desde time.Time
	if !o.Fin && len(servidor) > 0 {
		desde = s[len(s)-1].Tomada
	}
	var c []ElementoLinea[S]
	for _, f := range cifradas {
		t := f.Entrada.Tomada
		if !tope.IsZero() && !t.Before(tope) {
			continue
		}
		if !o.Busqueda && (ninguna || !desde.IsZero() && t.Before(desde)) {
			continue
		}
		c = append(c, ElementoLinea[S]{Cifrada: f, EsCifrada: true, Tomada: t})
	}
	if o.Busqueda {
		return append(c, s...)
	}
	out := make([]ElementoLinea[S], 0, len(s)+len(c))
	i, j := 0, 0
	for i < len(s) || j < len(c) {
		if j < len(c) && (i >= len(s) || c[j].Tomada.After(s[i].Tomada)) {
			out = append(out, c[j])
			j++
			continue
		}
		out = append(out, s[i])
		i++
	}
	return out
}

// Buscar son las fotos cuyo nombre, o la fecha que da `fecha` (el mes y el año en el
// idioma de quien mira, como la web), contienen `texto`, sin distinguir mayúsculas.
// La búsqueda es en el dispositivo: el servidor no sabe nada de estas fotos.
func Buscar(fotos []Foto, texto string, fecha func(time.Time) string) []Foto {
	q := strings.TrimSpace(texto)
	if q == "" {
		return fotos
	}
	var out []Foto
	for _, f := range fotos {
		nombre := f.Entrada.Nombre
		if nombre == "" {
			nombre = f.Fichero.Name
		}
		// Sin acentos ni mayúsculas, como toda búsqueda de la suite (buscar.Contiene).
		if buscar.Contiene(nombre, q) || fecha != nil && buscar.Contiene(fecha(f.Entrada.Tomada), q) {
			out = append(out, f)
		}
	}
	return out
}
