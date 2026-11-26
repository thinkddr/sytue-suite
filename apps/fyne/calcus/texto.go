// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// # Limpiar texto (`sheets/textoCeldas.ts`)
//
// Quitar los espacios que sobran y cambiar las mayúsculas del texto de las celdas
// elegidas, en su sitio: lo que pide cualquier dato pegado de otra parte, y que en una
// hoja solo se hacía con una columna auxiliar de `=ESPACIOS()`. Las mayúsculas son las
// de «Cambiar mayúsculas y minúsculas» de Docus (`rico.CambioMayusculas`), no otra regla,
// y los blancos, los de JavaScript (`editor.EsBlancoJS`): con los mismos vectores que la
// web (`packages/office/test/vectores/calcus-herramientas.json`).

// CmdLimpiarTexto es el comando `cleantext`; el valor es el modo.
const CmdLimpiarTexto editor.Comando = "cleantext"

// ModosTexto son «Recortar espacios» y los cuatro de las mayúsculas, en el orden del menú.
var ModosTexto = []string{"trim", "sentence", "lower", "upper", "title"}

// transformarTextoCelda es `transformarTextoCelda`: «trim» deja cada línea sin blancos en
// los extremos y con los de dentro de uno en uno, y quita las líneas vacías del principio
// y del final; los demás modos son los de las mayúsculas.
func transformarTextoCelda(texto, modo string) string {
	if modo != "trim" {
		m, ok := rico.LeerModoMayusculas(modo)
		if !ok {
			return texto
		}
		return rico.NuevoCambioMayusculas(m).Tramo(texto)
	}
	lineas := strings.Split(strings.ReplaceAll(strings.ReplaceAll(texto, "\r\n", "\n"), "\r", "\n"), "\n")
	for i, l := range lineas {
		lineas[i] = strings.Join(strings.FieldsFunc(l, editor.EsBlancoJS), " ")
	}
	for len(lineas) > 0 && lineas[0] == "" {
		lineas = lineas[1:]
	}
	for len(lineas) > 0 && lineas[len(lineas)-1] == "" {
		lineas = lineas[:len(lineas)-1]
	}
	return strings.Join(lineas, "\n")
}

// LimpiarTexto transforma el texto de los LITERALES del rango en UN paso de deshacer. Una
// fórmula calcula su texto y no se toca; una celda con marcado que este lienzo no entiende
// tampoco (`celdaEntendida`). Devuelve si cambió algo.
func (l *Libro) LimpiarTexto(modo string, c1, f1, c2, f2 int) bool {
	if !slices.Contains(ModosTexto, modo) {
		return false
	}
	nuevo := map[Celda]string{}
	for k, html := range l.h().celdas {
		if k.C < c1 || k.C > c2 || k.F < f1 || k.F > f2 || esFormula(html) || !celdaEntendida(html) {
			continue
		}
		antes := textoPlano(html)
		if despues := transformarTextoCelda(antes, modo); despues != antes {
			nuevo[k] = conTexto(html, despues)
		}
	}
	if len(nuevo) == 0 {
		return false
	}
	l.anotar()
	for k, html := range nuevo {
		l.poner(l.f.activa, k.C, k.F, html)
	}
	return true
}
