// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analisis

import (
	"cmp"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Los ESCENARIOS: `sheets/scenario.ts` de la web. Un escenario es un nombre y el valor
// (texto crudo) de unas celdas de entrada; el informe resumen los compara contra unas
// celdas de resultado.

// TopeEscenario es `SCENARIO_MAX_CELLS`: 32 celdas cambiantes, como Excel.
const TopeEscenario = 32

// Escenario es `Scenario`.
type Escenario struct {
	Name   string            `json:"name"`
	Values map[string]string `json:"values"`
}

// unaCelda dice si el texto es UNA celda A1 y la devuelve en mayúsculas.
func unaCelda(ref string) (string, bool) {
	r, ok := formula.ParseRangeRef(ref)
	if !ok || r.C1 != r.C2 || r.R1 != r.R2 {
		return "", false
	}
	return strings.ToUpper(strings.TrimSpace(ref)), true
}

// SanearEscenario es `sanitizeScenario`: nombre no vacío y claves de UNA celda con
// valor de texto, en mayúsculas. false si no queda nada.
func SanearEscenario(crudo any) (Escenario, bool) {
	m, ok := crudo.(map[string]any)
	if !ok {
		return Escenario{}, false
	}
	nombre, _ := m["name"].(string)
	vals, _ := m["values"].(map[string]any)
	if strings.TrimSpace(nombre) == "" || vals == nil {
		return Escenario{}, false
	}
	e := Escenario{Name: strings.TrimSpace(nombre), Values: map[string]string{}}
	for ref, v := range vals {
		s, esTexto := v.(string)
		if k, ok := unaCelda(ref); ok && esTexto {
			e.Values[k] = s
		}
	}
	return e, len(e.Values) > 0
}

// RotulosInforme son los textos del informe, ya traducidos.
type RotulosInforme struct {
	Titulo, Cambiantes, Resultados string
	// Actual es el rótulo de la columna de valores actuales; vacío, sin esa columna.
	Actual string
}

// ordenLectura son las celdas en orden de lectura: fila y luego columna.
func ordenLectura(refs []string) []string {
	type par struct {
		ref string
		r   formula.Rect
	}
	var ps []par
	for _, ref := range refs {
		if r, ok := formula.ParseRangeRef(ref); ok {
			ps = append(ps, par{ref, r})
		}
	}
	slices.SortStableFunc(ps, func(a, b par) int { return cmp.Or(cmp.Compare(a.r.R1, b.r.R1), cmp.Compare(a.r.C1, b.r.C1)) })
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ref
	}
	return out
}

// entradasDe es la unión de las celdas que cambian los escenarios, en orden de lectura.
func entradasDe(esc []Escenario) []string {
	var union []string
	visto := map[string]bool{}
	for _, e := range esc {
		// El orden de un objeto JS es el de inserción; aquí se ordena igual al final.
		claves := make([]string, 0, len(e.Values))
		for k := range e.Values {
			claves = append(claves, k)
		}
		slices.Sort(claves)
		for _, k := range claves {
			if !visto[k] {
				visto[k] = true
				union = append(union, k)
			}
		}
	}
	return ordenLectura(union)
}

// resultadosDe son las celdas de resultado válidas, sin repetir.
func resultadosDe(resultados []string) []string {
	var res []string
	visto := map[string]bool{}
	for _, ref := range resultados {
		if k, ok := unaCelda(ref); ok && !visto[k] {
			visto[k] = true
			res = append(res, k)
		}
	}
	return res
}

// columnaInforme es un escenario del informe; fijar nil = los valores actuales.
type columnaInforme struct {
	cabecera string
	fijar    map[string]string
}

// InformeEscenarios es `buildScenarioReport`: la rejilla de texto que compara los
// escenarios. `evaluar` fija los valores dados y devuelve el texto de las celdas de
// resultado pedidas; `actual` son los crudos actuales de las celdas de entrada.
func InformeEscenarios(esc []Escenario, resultados []string, evaluar func(fijar map[string]string, refs []string) map[string]string,
	r RotulosInforme, actual map[string]string,
) [][]string {
	entradas, res := entradasDe(esc), resultadosDe(resultados)
	cols := make([]columnaInforme, 0, 1+len(esc))
	if r.Actual != "" {
		cols = append(cols, columnaInforme{cabecera: r.Actual})
	}
	for _, e := range esc {
		cols = append(cols, columnaInforme{cabecera: e.Name, fijar: e.Values})
	}
	porCol := make([]map[string]string, len(cols))
	for i, c := range cols {
		porCol[i] = map[string]string{}
		if len(res) > 0 {
			f := c.fijar
			if f == nil {
				f = map[string]string{}
			}
			porCol[i] = evaluar(f, res)
		}
	}
	fila := func(primera string, resto func(i int, c columnaInforme) string) []string {
		out := make([]string, 0, 1+len(cols))
		out = append(out, primera)
		for i, c := range cols {
			out = append(out, resto(i, c))
		}
		return out
	}
	vacio := func(int, columnaInforme) string { return "" }
	out := [][]string{fila(r.Titulo, func(_ int, c columnaInforme) string { return c.cabecera })}
	if len(entradas) > 0 {
		out = append(out, fila(r.Cambiantes, vacio))
		for _, ref := range entradas {
			out = append(out, fila(ref, func(_ int, c columnaInforme) string {
				if v, ok := c.fijar[ref]; ok {
					return v
				}
				return actual[ref]
			}))
		}
	}
	if len(res) > 0 {
		out = append(out, fila(r.Resultados, vacio))
		for _, ref := range res {
			out = append(out, fila(ref, func(i int, _ columnaInforme) string { return porCol[i][ref] }))
		}
	}
	return out
}
