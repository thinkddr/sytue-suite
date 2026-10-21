// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// Modelo documental por tipo, construido SOBRE las primitivas CRDT comunes
// (secuencia RGA + registros LWW de crdt.go). Aquí viven los ayudantes que dan
// forma a cada producto de la suite ofimática:
//
//   - Document      -> secuencia RGA de caracteres (texto) + marcas de formato LWW.
//   - Spreadsheet   -> registros LWW por celda, con clave canónica "Hoja!fila:col"
//                      y conversión a/desde referencia A1 (p. ej. "Hoja1!B3").
//   - Presentation  -> secuencia RGA de diapositivas + registros LWW de propiedades.
//
// Convención de claves de registro LWW (interpretable por backend y frontend):
//
//   celda de hoja           ->  "cell:<hoja>!<fila>:<col>"        (1-indexado)
//   marca de formato texto  ->  "fmt:<dotActor>:<dotCounter>:<k>" (por carácter)
//   propiedad diapositiva   ->  "slide:<slideDotActor>:<n>:<k>"
//   propiedad de documento  ->  "doc:<k>"                         (título interno, tema…)
//
// Estas funciones son puras y deterministas; no dependen de estado externo. Son
// obra original de Sytue (clean-room) sin código de terceros.

// --- Hoja de cálculo: celdas y referencias A1 ---

// CellKey construye la clave de registro LWW canónica de una celda a partir de
// (hoja, fila, columna) con índices 1-basados. Filas/columnas <= 0 se saturan a 1
// para no producir claves inválidas.
func CellKey(sheet string, row, col int) string {
	if row < 1 {
		row = 1
	}
	if col < 1 {
		col = 1
	}
	if sheet == "" {
		sheet = "Hoja1"
	}
	return fmt.Sprintf("cell:%s!%d:%d", sheet, row, col)
}

// ParseCellKey descompone una clave "cell:<hoja>!<fila>:<col>" en sus partes.
// Devuelve ok=false si la clave no es una celda válida.
func ParseCellKey(key string) (sheet string, row, col int, ok bool) {
	rest, found := strings.CutPrefix(key, "cell:")
	if !found {
		return "", 0, 0, false
	}
	bang := strings.LastIndexByte(rest, '!')
	if bang < 0 {
		return "", 0, 0, false
	}
	sheet = rest[:bang]
	rc := rest[bang+1:]
	colon := strings.IndexByte(rc, ':')
	if colon < 0 {
		return "", 0, 0, false
	}
	row, err1 := strconv.Atoi(rc[:colon])
	col, err2 := strconv.Atoi(rc[colon+1:])
	if err1 != nil || err2 != nil || row < 1 || col < 1 || sheet == "" {
		return "", 0, 0, false
	}
	return sheet, row, col, true
}

// ColumnToLetters convierte un índice de columna 1-basado a su etiqueta A1
// (1->A, 26->Z, 27->AA, 702->ZZ, 703->AAA). Es la base bijectiva 26 usada por
// OOXML/ODF para nombrar columnas de hoja de cálculo.
func ColumnToLetters(col int) string {
	if col < 1 {
		col = 1
	}
	var b []byte
	for col > 0 {
		col--
		b = append([]byte{byte('A' + col%26)}, b...)
		col /= 26
	}
	return string(b)
}

// LettersToColumn convierte una etiqueta de columna A1 ("A", "AA"…) al índice
// 1-basado. Devuelve 0 si la etiqueta contiene caracteres no A–Z.
func LettersToColumn(letters string) int {
	if letters == "" {
		return 0
	}
	col := 0
	for i := 0; i < len(letters); i++ {
		c := letters[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c < 'A' || c > 'Z' {
			return 0
		}
		col = col*26 + int(c-'A'+1)
	}
	return col
}

// A1Ref convierte (fila, columna) 1-basados a una referencia A1 ("B3").
func A1Ref(row, col int) string {
	if row < 1 {
		row = 1
	}
	return ColumnToLetters(col) + strconv.Itoa(row)
}

// ParseA1 descompone una referencia A1 ("B3", "AA10") en (fila, columna)
// 1-basados. Devuelve ok=false si no es una referencia A1 válida.
func ParseA1(ref string) (row, col int, ok bool) {
	i := 0
	for i < len(ref) {
		c := ref[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			i++
			continue
		}
		break
	}
	if i == 0 || i == len(ref) {
		return 0, 0, false
	}
	col = LettersToColumn(ref[:i])
	if col == 0 {
		return 0, 0, false
	}
	row, err := strconv.Atoi(ref[i:])
	if err != nil || row < 1 {
		return 0, 0, false
	}
	return row, col, true
}

// CellA1 devuelve la referencia A1 con nombre de hoja ("Hoja1!B3") de una clave
// de celda. Devuelve "" si la clave no es una celda válida.
func CellA1(key string) string {
	sheet, row, col, ok := ParseCellKey(key)
	if !ok {
		return ""
	}
	return sheet + "!" + A1Ref(row, col)
}

// --- Documento de texto: marcas de formato por carácter ---

// FormatMarkKey construye la clave LWW de una marca de formato (negrita,
// cursiva, color…) anclada a un carácter concreto (identificado por su Dot). El
// formato es LWW, independiente de la secuencia RGA del texto: sobrevive a
// inserciones/borrados vecinos porque referencia la identidad estable del carácter.
func FormatMarkKey(char Dot, mark string) string {
	return fmt.Sprintf("fmt:%s:%d:%s", char.Actor, char.Counter, mark)
}

// --- Presentación: propiedades de diapositiva ---

// SlidePropKey construye la clave LWW de una propiedad (título, fondo, layout…)
// de una diapositiva, identificada por el Dot de su elemento de secuencia RGA.
func SlidePropKey(slide Dot, prop string) string {
	return fmt.Sprintf("slide:%s:%d:%s", slide.Actor, slide.Counter, prop)
}

// DocPropKey construye la clave LWW de una propiedad global del documento
// (tema, dirección de texto, idioma…). No está anclada a ningún elemento.
func DocPropKey(prop string) string { return "doc:" + prop }
