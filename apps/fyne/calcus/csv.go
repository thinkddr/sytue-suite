// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"regexp"
	"strings"
)

// # Descargar la hoja como CSV (`sheets/csv.ts`)
//
// La hoja ACTIVA desde A1 hasta lo último con algo, con lo que cada celda ENSEÑA —lo mismo
// que copiar—, en el CSV del RFC 4180: coma, comillas dobles donde hacen falta y CRLF.
// Delante va la marca de orden de bytes de UTF-8, sin la cual Excel lee «Año» como «AÃ±o».
// Se escribe aquí, sin pedir nada al servicio, como en la web; con sus mismos vectores
// (`packages/office/test/vectores/calcus-herramientas.json`).

// BOMUTF8 es la marca de orden de bytes de UTF-8.
const BOMUTF8 = "\ufeff"

// campoCSV va entre comillas si lleva coma, comillas o un salto de línea.
func campoCSV(v string) string {
	if strings.ContainsAny(v, "\",\r\n") {
		return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
	}
	return v
}

// csvDeFilas es `csvDeFilas`: las filas igualadas al ancho de la más larga, sin la marca.
func csvDeFilas(filas [][]string) string {
	ancho := 0
	for _, f := range filas {
		ancho = max(ancho, len(f))
	}
	var b strings.Builder
	for _, f := range filas {
		for i := range ancho {
			if i > 0 {
				b.WriteByte(',')
			}
			if i < len(f) {
				b.WriteString(campoCSV(f[i]))
			}
		}
		b.WriteString("\r\n")
	}
	return b.String()
}

var reNombreHoja = regexp.MustCompile(`[\\/:*?"<>|]+`)

// nombreCSV es «Título - Hoja.csv», como Sheets. `base` ya va saneado.
func nombreCSV(base, hoja string) string {
	limpio := strings.TrimSpace(reNombreHoja.ReplaceAllString(hoja, "_"))
	if limpio == "" {
		return base + ".csv"
	}
	return base + " - " + limpio + ".csv"
}

// FilasCSV son las filas de la hoja activa tal y como se ven, desde A1 hasta la última fila
// y columna con algo: las vacías de delante también, o el CSV no volvería a caer en su sitio.
func (l *Libro) FilasCSV() [][]string {
	ultC, ultF := -1, -1
	for k, html := range l.h().celdas {
		if html != "" {
			ultC, ultF = max(ultC, k.C), max(ultF, k.F)
		}
	}
	filas := make([][]string, 0, ultF+1)
	for f := 0; f <= ultF; f++ {
		fila := make([]string, ultC+1)
		for c := range fila {
			fila[c] = l.Texto(c, f)
		}
		filas = append(filas, fila)
	}
	return filas
}

// CSV es el fichero entero de la hoja activa, con la marca delante.
func (l *Libro) CSV() []byte { return []byte(BOMUTF8 + csvDeFilas(l.FilasCSV())) }
