// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Abrir un PDF como documento editable. Es la ola P6 de Sytue PDF —convertir— y
// sale casi entera de aquí: en cuanto un PDF entra por el import, el resto del
// camino YA existe. Exportarlo a Word, a ODT, a HTML o a Markdown es lo que Docs
// hace con cualquier documento, así que «PDF a Word» no necesita un exportador
// nuevo, necesita esta puerta.
//
// La conversión reconstruye párrafos a partir del texto con posiciones que da
// `pdftext.go`: se agrupan los fragmentos por línea, las líneas por bloque y los
// bloques se emiten como <p>. Un PDF no guarda párrafos —guarda dónde se pinta
// cada trozo de texto—, así que esto es una RECONSTRUCCIÓN y no una lectura.
//
// El límite, dicho aquí y no descubierto por el usuario a mitad de trabajo: se
// recupera el TEXTO y su orden, no la maquetación. Columnas, tablas, imágenes y
// tipografías no sobreviven; un PDF escaneado (solo imágenes) no tiene texto que
// recuperar y sale vacío hasta que exista OCR. Para revisar y reutilizar el
// contenido sirve; para reproducir el documento tal cual, no.
package domain

import (
	"fmt"
	"html"
	"math"
	"sort"
	"strings"
)

// importPDF convierte un PDF en el HTML inicial de un documento de texto.
func importPDF(data []byte) (Kind, string, error) {
	doc, err := LeerPDF(data)
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", ErrInvalidFormat, err)
	}

	var b strings.Builder
	for i := 0; i < doc.NumPaginas(); i++ {
		frags, err := doc.TextoDePagina(i)
		if err != nil {
			continue // una página ilegible no invalida el documento entero
		}
		if i > 0 {
			// Separador de página: un PDF no tiene «salto de página» que
			// traducir, pero perder por completo dónde acababa cada una deja un
			// muro de texto imposible de revisar.
			b.WriteString("<hr>\n")
		}
		for _, p := range parrafosDeFragmentos(frags) {
			b.WriteString("<p>" + html.EscapeString(p) + "</p>\n")
		}
	}

	if strings.TrimSpace(b.String()) == "" {
		// Un PDF escaneado no es un error del usuario ni del fichero: es que
		// todavía no sabemos leerlo. Decirlo es mejor que devolver un documento
		// vacío que parece un fallo.
		return "", "", fmt.Errorf("%w: el PDF no tiene texto que recuperar "+
			"(puede ser un escaneo; hará falta OCR)", ErrInvalidFormat)
	}
	return KindDocument, b.String(), nil
}

// parrafosDeFragmentos reconstruye los párrafos de una página.
//
// Dos agrupaciones, y el orden importa:
//
//  1. **Por línea.** Los fragmentos que comparten altura son la misma línea, aunque
//     vengan en varios operadores de texto (un cambio de fuente parte la línea en
//     dos). Se ordenan por X, que es como se leen.
//  2. **Por párrafo.** Un salto vertical mayor que el interlineado normal separa
//     párrafos. El umbral se calcula del propio documento y no se fija a un número:
//     un PDF a doble espacio y otro compacto tienen interlineados muy distintos, y
//     una constante acertaría en uno y partiría cada línea del otro.
func parrafosDeFragmentos(frags []PDFFragmento) []string {
	// Fuera lo que no aporta: los fragmentos vacíos ensucian el cálculo del
	// interlineado sin añadir texto.
	vivos := make([]PDFFragmento, 0, len(frags))
	for _, f := range frags {
		if strings.TrimSpace(f.Texto) != "" {
			vivos = append(vivos, f)
		}
	}
	if len(vivos) == 0 {
		return nil
	}

	// 1. Agrupar por línea. En PDF la Y crece hacia ARRIBA, así que se ordena
	// descendente para recorrer la página de arriba abajo.
	sort.SliceStable(vivos, func(i, j int) bool {
		if math.Abs(vivos[i].Y-vivos[j].Y) > 0.5 {
			return vivos[i].Y > vivos[j].Y
		}
		return vivos[i].X < vivos[j].X
	})

	var lineas []lineaPDF
	for _, f := range vivos {
		n := len(lineas)
		if n > 0 && math.Abs(lineas[n-1].y-f.Y) <= 0.5 {
			lineas[n-1].trozo = append(lineas[n-1].trozo, f.Texto)
			continue
		}
		lineas = append(lineas, lineaPDF{y: f.Y, trozo: []string{f.Texto}})
	}

	// 2. Umbral de párrafo a partir del interlineado MEDIANO de esta página. La
	// mediana y no la media: un título grande o un salto entre bloques
	// desplazarían la media y romperían el resto.
	umbral := umbralDeParrafo(saltosEntreLineas(lineas))

	var out []string
	var actual []string
	for i, l := range lineas {
		if i > 0 {
			salto := lineas[i-1].y - l.y
			if salto > umbral {
				if p := juntar(actual); p != "" {
					out = append(out, p)
				}
				actual = nil
			}
		}
		actual = append(actual, strings.Join(l.trozo, " "))
	}
	if p := juntar(actual); p != "" {
		out = append(out, p)
	}
	return out
}

// lineaPDF es una línea de texto reconstruida: su altura y sus trozos en orden.
type lineaPDF struct {
	y     float64
	trozo []string
}

// saltosEntreLineas devuelve las distancias verticales entre líneas consecutivas.
func saltosEntreLineas(lineas []lineaPDF) []float64 {
	if len(lineas) < 2 {
		return nil
	}
	out := make([]float64, 0, len(lineas)-1)
	for i := 1; i < len(lineas); i++ {
		if d := lineas[i-1].y - lineas[i].y; d > 0 {
			out = append(out, d)
		}
	}
	return out
}

// umbralDeParrafo: un 50 % por encima del interlineado mediano. Por debajo de
// eso es la línea siguiente del mismo párrafo; por encima, un párrafo nuevo.
func umbralDeParrafo(saltos []float64) float64 {
	if len(saltos) == 0 {
		return math.MaxFloat64 // una sola línea: todo es un párrafo
	}
	orden := append([]float64(nil), saltos...)
	sort.Float64s(orden)
	mediana := orden[len(orden)/2]
	if mediana <= 0 {
		return math.MaxFloat64
	}
	return mediana * 1.5
}

// juntar une las líneas de un párrafo en una sola cadena, cosiendo las palabras
// partidas con guion al final de línea, que es como se rompe un texto justificado.
func juntar(lineas []string) string {
	var b strings.Builder
	for i, l := range lineas {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if i > 0 && b.Len() > 0 {
			actual := b.String()
			if strings.HasSuffix(actual, "-") {
				// Palabra partida: se quita el guion y se pega sin espacio.
				b.Reset()
				b.WriteString(strings.TrimSuffix(actual, "-"))
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(l)
	}
	return strings.TrimSpace(b.String())
}
