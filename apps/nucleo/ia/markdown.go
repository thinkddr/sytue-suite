// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ia es lo que las apps nativas hacen con la respuesta de la IA sin depender del
// toolkit: el puerto de `packages/ai/src/markdown.ts`.
//
// El Markdown del modelo se convierte en BLOQUES NEUTROS y no en HTML: es texto de un
// tercero, y cada app lo pinta con sus propias piezas. Subconjunto deliberado: títulos,
// listas, negrita y cursiva; lo que no se reconoce se queda como texto (peor formato,
// nunca contenido perdido).
package ia

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Trozo es un tramo de texto con su énfasis.
type Trozo struct {
	Texto   string
	Negrita bool
	Cursiva bool
}

// Tipo es el de un bloque: `p`, `h1`, `h2`, `h3` o `li`.
type Tipo string

// Los tipos de bloque.
const (
	Parrafo Tipo = "p"
	H1      Tipo = "h1"
	H2      Tipo = "h2"
	H3      Tipo = "h3"
	Punto   Tipo = "li"
)

// Bloque es una línea del texto con su tipo. Ordenada solo vale en `li`: si el punto
// venía numerado.
type Bloque struct {
	Tipo     Tipo
	Trozos   []Trozo
	Ordenada bool
}

var (
	vineta   = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)
	numerada = regexp.MustCompile(`^\s*\d{1,3}[.)]\s+(.*)$`)
	titulo   = regexp.MustCompile(`^\s*(#{1,3})\s+(.*)$`)
	enfasis  = regexp.MustCompile(`\*\*(.+?)\*\*|__(.+?)__|\*(.+?)\*|_(.+?)_`)
)

// TrozosDe parte una línea según `**negrita**`, `*cursiva*` y `_cursiva_`, en una pasada
// y sin anidar. Los marcadores impares no se comen: `2 * 3 = 6` conserva su asterisco.
func TrozosDe(linea string) []Trozo {
	var salida []Trozo
	ultimo := 0
	for _, m := range enfasis.FindAllStringSubmatchIndex(linea, -1) {
		if m[0] > ultimo {
			salida = append(salida, Trozo{Texto: linea[ultimo:m[0]]})
		}
		switch {
		case m[2] >= 0:
			salida = append(salida, Trozo{Texto: linea[m[2]:m[3]], Negrita: true})
		case m[4] >= 0:
			salida = append(salida, Trozo{Texto: linea[m[4]:m[5]], Negrita: true})
		case m[6] >= 0:
			salida = append(salida, Trozo{Texto: linea[m[6]:m[7]], Cursiva: true})
		case m[8] >= 0:
			salida = append(salida, Trozo{Texto: linea[m[8]:m[9]], Cursiva: true})
		}
		ultimo = m[1]
	}
	if ultimo < len(linea) {
		salida = append(salida, Trozo{Texto: linea[ultimo:]})
	}
	r := salida[:0]
	for _, t := range salida {
		if t.Texto != "" {
			r = append(r, t)
		}
	}
	return r
}

// BloquesDe convierte el texto del modelo en bloques. Las líneas en blanco separan y no
// producen ninguno.
func BloquesDe(texto string) []Bloque {
	var bloques []Bloque
	texto = strings.ReplaceAll(strings.ReplaceAll(texto, "\r\n", "\n"), "\r", "\n")
	for _, cruda := range strings.Split(texto, "\n") {
		linea := strings.TrimRightFunc(cruda, unicode.IsSpace)
		if strings.TrimSpace(linea) == "" {
			continue
		}
		if m := titulo.FindStringSubmatch(linea); m != nil {
			tipo := map[int]Tipo{1: H1, 2: H2, 3: H3}[len(m[1])]
			bloques = append(bloques, Bloque{Tipo: tipo, Trozos: TrozosDe(m[2])})
			continue
		}
		if m := numerada.FindStringSubmatch(linea); m != nil {
			bloques = append(bloques, Bloque{Tipo: Punto, Ordenada: true, Trozos: TrozosDe(m[1])})
			continue
		}
		if m := vineta.FindStringSubmatch(linea); m != nil {
			bloques = append(bloques, Bloque{Tipo: Punto, Trozos: TrozosDe(m[1])})
			continue
		}
		// UN PÁRRAFO QUE ES SOLO NEGRITA ES UN TÍTULO: los modelos escriben
		// `**Introducción**` en vez de `## Introducción` la mitad de las veces.
		trozos := TrozosDe(linea)
		if len(trozos) == 1 && trozos[0].Negrita && utf8.RuneCountInString(trozos[0].Texto) <= 80 {
			bloques = append(bloques, Bloque{Tipo: H2, Trozos: []Trozo{{Texto: trozos[0].Texto}}})
			continue
		}
		bloques = append(bloques, Bloque{Tipo: Parrafo, Trozos: trozos})
	}
	return bloques
}

// TextoPlano es el texto pelado de los bloques, sin marcadores, con «• » en las listas.
func TextoPlano(bloques []Bloque) string {
	lineas := make([]string, len(bloques))
	for i, b := range bloques {
		var s strings.Builder
		for _, t := range b.Trozos {
			s.WriteString(t.Texto)
		}
		lineas[i] = s.String()
		if b.Tipo == Punto {
			lineas[i] = "• " + lineas[i]
		}
	}
	return strings.Join(lineas, "\n")
}
