// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ancla valida el ancla de un comentario de un documento: la misma regla para el
// servicio docs (que la recibe del cliente) y para el modo privado de las apps nativas (que
// guarda los comentarios en local sin pasar por el servicio). Vive en el núcleo porque el
// servicio ya lo importa y el núcleo no puede importar el `internal/` del servicio; si cada
// uno tuviera la suya, el modo privado aceptaría anclas que el servicio rechaza y el libro
// dejaría de poder sincronizarse o compartirse igual.
package ancla

import (
	"encoding/json"
	"math"
	"regexp"
	"unicode/utf8"
)

// Max acota el ancla de un comentario: es JSON libre del cliente y se devuelve a todos los
// lectores del documento.
const Max = 4096

// maxTexto acota la hoja y el texto citado de un ancla.
const maxTexto = 300

var reID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// reMarca es el id de un <mark data-comment-id>: hex, porque el editor lo usa sin escapar en un
// selector.
var reMarca = regexp.MustCompile(`^[0-9a-f]{1,64}$`)

// EsID dice si s es un id de ancla válido (de una fila, una columna, un dibujo o un nodo).
func EsID(s string) bool { return reID.MatchString(s) }

// EsMarca dice si s es un id válido de un <mark data-comment-id> de un comentario importado.
func EsMarca(s string) bool { return reMarca.MatchString(s) }

// DeCelda es el ancla de un comentario de una celda importado (la hoja, la identidad de su fila
// y de su columna y la referencia citada), o nil si no es válida.
func DeCelda(hoja, fila, col, texto string) map[string]string {
	if hoja == "" || utf8.RuneCountInString(hoja) > maxTexto || utf8.RuneCountInString(texto) > maxTexto ||
		!EsID(fila) || !EsID(col) {
		return nil
	}
	return map[string]string{"hoja": hoja, "fila": fila, "col": col, "text": texto}
}

// Validar comprueba el ancla de un comentario y devuelve el motivo del rechazo, o "" si vale.
// Vacía o `null` es un comentario sin ancla; el resto ha de ser un objeto pequeño. Si lleva
// `dibujo` (un comentario sobre un dibujo de Docus) se valida su forma: `{dibujo, nodo | x,y,
// text}`, con `nodo` el id de una forma de la escena o (x, y) una coordenada del lienzo, nunca las
// dos cosas. Si lleva `hoja` (un comentario de una celda de Calcus, `sheets/anclaCelda.ts`) es
// `{hoja, fila, col, text}`: el id de la hoja y la identidad de su fila y su columna. Las demás
// anclas (`{mark, text}`) no se interpretan aquí.
func Validar(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if len(raw) > Max {
		return "The comment anchor is too large."
	}
	var a map[string]json.RawMessage
	if json.Unmarshal(raw, &a) != nil {
		return "The comment anchor must be an object."
	}
	if _, ok := a["hoja"]; ok {
		return validarCelda(a)
	}
	if _, ok := a["dibujo"]; !ok {
		return ""
	}
	for _, k := range []string{"dibujo", "nodo"} {
		if v, ok := a[k]; ok {
			var s string
			if json.Unmarshal(v, &s) != nil || !EsID(s) {
				return "The comment anchor has an invalid " + k + "."
			}
		}
	}
	_, conNodo := a["nodo"]
	_, conX := a["x"]
	_, conY := a["y"]
	if conX != conY || (conNodo && conX) {
		return "The comment anchor must have a node or a point, not both."
	}
	return validarCoordenadas(a)
}

// validarCelda es la forma del ancla de una celda: `hoja` un texto corto, `fila` y `col` ids de
// eje (`t3`, `n1a2b3c4d`) y `text` la referencia citada. Ni marca ni dibujo a la vez.
func validarCelda(a map[string]json.RawMessage) string {
	var hoja string
	if json.Unmarshal(a["hoja"], &hoja) != nil || hoja == "" || utf8.RuneCountInString(hoja) > maxTexto {
		return "The comment anchor has an invalid hoja."
	}
	for _, k := range []string{"fila", "col"} {
		var s string
		if json.Unmarshal(a[k], &s) != nil || !EsID(s) {
			return "The comment anchor has an invalid " + k + "."
		}
	}
	for _, k := range []string{"mark", "dibujo", "nodo", "x", "y"} {
		if _, ok := a[k]; ok {
			return "A cell comment anchor cannot have " + k + "."
		}
	}
	return validarCoordenadas(a)
}

// validarCoordenadas comprueba `x`, `y` y `text` de un ancla de dibujo o de celda.
func validarCoordenadas(a map[string]json.RawMessage) string {
	for _, k := range []string{"x", "y"} {
		if v, ok := a[k]; ok {
			var f float64
			if json.Unmarshal(v, &f) != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 1e6 {
				return "The comment anchor has an invalid " + k + "."
			}
		}
	}
	if v, ok := a["text"]; ok {
		var s string
		if json.Unmarshal(v, &s) != nil || utf8.RuneCountInString(s) > maxTexto {
			return "The comment anchor has an invalid text."
		}
	}
	return ""
}
