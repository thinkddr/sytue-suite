// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"slices"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// # Autosuma (`sheets/autosuma.ts`)
//
// Σ en la pestaña Fórmulas y Alt+=: la fórmula que se escribe sola al pie de una columna
// de números. El PLAN —qué celdas reciben fórmula y sobre qué rango— es el de la web, con
// sus mismos vectores (`packages/office/test/vectores/calcus-herramientas.json`): la misma
// selección no puede sumar otro rango según dónde se pulse.

// CmdAutosuma es el comando `autosum`; el valor es la función ("SUM" si va vacío).
const CmdAutosuma editor.Comando = "autosum"

// FuncionesAutosuma son las del desplegable de Σ, en el orden de Excel.
var FuncionesAutosuma = []string{"SUM", "AVERAGE", "COUNT", "MAX", "MIN"}

// totalAutosuma es una fórmula del plan: dónde se escribe y el rango que resume.
type totalAutosuma struct {
	celda          Celda
	c1, f1, c2, f2 int
}

// planDeAutosuma es `planDeAutosuma`: nil y false es una celda sola sin números al lado
// (se abre con `=SUM(`); un plan vacío con true, nada que escribir.
func planDeAutosuma(c1, f1, c2, f2 int, esNumero, vacia func(c, f int) bool, cols, filas int) ([]totalAutosuma, bool) {
	if c1 == c2 && f1 == f2 {
		return autosumaDeCelda(c1, f1, esNumero)
	}
	if f2 > f1 {
		return autosumaPorColumnas(c1, f1, c2, f2, esNumero, vacia, filas), true
	}
	return autosumaDeFila(c1, f1, c2, esNumero, vacia, cols), true
}

// autosumaDeCelda son los números seguidos de encima o, si no hay, los de la izquierda.
func autosumaDeCelda(c1, f1 int, esNumero func(c, f int) bool) ([]totalAutosuma, bool) {
	k := f1 - 1
	for k >= 0 && esNumero(c1, k) {
		k--
	}
	if k < f1-1 {
		return []totalAutosuma{{Celda{C: c1, F: f1}, c1, k + 1, c1, f1 - 1}}, true
	}
	k = c1 - 1
	for k >= 0 && esNumero(k, f1) {
		k--
	}
	if k < c1-1 {
		return []totalAutosuma{{Celda{C: c1, F: f1}, k + 1, f1, c1 - 1, f1}}, true
	}
	return nil, false
}

// autosumaPorColumnas es un total por columna con algún número, en la última fila si está
// vacía entera y si no en la de debajo.
func autosumaPorColumnas(c1, f1, c2, f2 int, esNumero, vacia func(c, f int) bool, filas int) []totalAutosuma {
	ultimaVacia := true
	for c := c1; c <= c2 && ultimaVacia; c++ {
		ultimaVacia = vacia(c, f2)
	}
	destino, hasta := f2+1, f2
	if ultimaVacia {
		destino, hasta = f2, f2-1
	}
	out := []totalAutosuma{}
	if destino >= filas {
		return out
	}
	for c := c1; c <= c2; c++ {
		for f := f1; f <= hasta; f++ {
			if esNumero(c, f) {
				out = append(out, totalAutosuma{Celda{C: c, F: destino}, c, f1, c, hasta})
				break
			}
		}
	}
	return out
}

// autosumaDeFila es un total a la derecha, con el mismo criterio de la última columna.
func autosumaDeFila(c1, f1, c2 int, esNumero, vacia func(c, f int) bool, cols int) []totalAutosuma {
	destino, hasta := c2+1, c2
	if vacia(c2, f1) {
		destino, hasta = c2, c2-1
	}
	if destino >= cols {
		return []totalAutosuma{}
	}
	for c := c1; c <= hasta; c++ {
		if esNumero(c, f1) {
			return []totalAutosuma{{Celda{C: destino, F: f1}, c1, f1, hasta, f1}}
		}
	}
	return []totalAutosuma{}
}

// formulaDeAutosuma es «=SUM(B2:B9)», o «=SUM(B2)» si el rango es una celda.
func formulaDeAutosuma(fn string, c1, f1, c2, f2 int) string {
	desde, hasta := Ref(c1, f1), Ref(c2, f2)
	if desde == hasta {
		return "=" + fn + "(" + desde + ")"
	}
	return "=" + fn + "(" + desde + ":" + hasta + ")"
}

// Autosuma escribe las fórmulas del plan en UN paso de deshacer. Devuelve false si la
// selección no tiene números al lado: entonces quien llama abre la celda con la función.
func (l *Libro) Autosuma(fn string, c1, f1, c2, f2 int) bool {
	if !slices.Contains(FuncionesAutosuma, fn) {
		fn = "SUM"
	}
	esNumero := func(c, f int) bool {
		_, ok := l.Valor(c, f).(float64)
		return ok
	}
	vacia := func(c, f int) bool { return l.HTML(c, f) == "" }
	plan, ok := planDeAutosuma(c1, f1, c2, f2, esNumero, vacia, MaxColumnas, MaxFilas)
	if !ok {
		return false
	}
	if len(plan) == 0 {
		return true
	}
	l.anotar()
	for _, t := range plan {
		l.poner(l.f.activa, t.celda.C, t.celda.F, conTexto(l.HTML(t.celda.C, t.celda.F), formulaDeAutosuma(fn, t.c1, t.f1, t.c2, t.f2)))
	}
	return true
}
