// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"html"
	"slices"
	"strconv"
	"strings"
)

// Los ESCENARIOS de una hoja de Calcus (`SheetData.scenarios`, `Scenario` de `types.ts`): un
// nombre y el valor (texto crudo) que toma cada celda de entrada. En OOXML son
// `<scenarios><scenario><inputCells r val/>`; en ODF, una hoja oculta por escenario con un
// `table:scenario` y los valores en las mismas celdas que la hoja base.

const (
	maxEscenarios       = 200
	maxValoresEscenario = 500
)

// escenariosHoja son los escenarios de una SheetData; van embebidos en sheetData.
type escenariosHoja struct {
	Scenarios []sheetScenario `json:"scenarios,omitempty"`
}

type sheetScenario struct {
	Name   string            `json:"name"`
	Values map[string]string `json:"values"`
}

// sanearEscenarios deja los escenarios con nombre, sin repetir y con al menos un valor en una
// celda A1; los valores en MAYÚSCULAS, como los guarda el editor.
func sanearEscenarios(es []sheetScenario) []sheetScenario {
	var out []sheetScenario
	for _, e := range es {
		nombre := strings.TrimSpace(e.Name)
		if nombre == "" || len(out) >= maxEscenarios || slices.ContainsFunc(out, func(x sheetScenario) bool { return x.Name == nombre }) {
			continue
		}
		vals := map[string]string{}
		for ref, v := range e.Values {
			if r, c, ok := ParseA1(strings.ToUpper(ref)); ok && r <= maxFilasHoja && c <= maxColumnasHoja && len(vals) < maxValoresEscenario {
				vals[A1Ref(r, c)] = v
			}
		}
		if len(vals) > 0 {
			out = append(out, sheetScenario{Name: nombre, Values: vals})
		}
	}
	return out
}

// refsOrdenadas son las celdas de un escenario en orden de lectura (fila y luego columna).
func (e sheetScenario) refsOrdenadas() []string {
	refs := make([]string, 0, len(e.Values))
	for r := range e.Values {
		refs = append(refs, r)
	}
	slices.SortFunc(refs, func(a, b string) int {
		ra, ca, _ := ParseA1(a)
		rb, cb, _ := ParseA1(b)
		if ra != rb {
			return ra - rb
		}
		return ca - cb
	})
	return refs
}

// escenariosXlsx son los `<scenarios>` de la hoja, justo tras `sheetData` (y la protección);
// "" si no hay.
func escenariosXlsx(sd sheetData) string {
	es := sanearEscenarios(sd.Scenarios)
	if len(es) == 0 {
		return ""
	}
	var todas []string
	var b strings.Builder
	for _, e := range es {
		refs := e.refsOrdenadas()
		todas = append(todas, refs...)
		b.WriteString(`<scenario name="` + xmlEscape(truncarRunas(e.Name, 255)) + `" count="` + strconv.Itoa(len(refs)) + `" user="">`)
		for _, r := range refs {
			b.WriteString(`<inputCells r="` + r + `" val="` + xmlEscape(e.Values[r]) + `"/>`)
		}
		b.WriteString(`</scenario>`)
	}
	slices.Sort(todas)
	todas = slices.Compact(todas)
	return `<scenarios current="0" show="0" sqref="` + strings.Join(todas, " ") + `">` + b.String() + `</scenarios>`
}

// xlsxEscenarios es el `<scenarios>` de una hoja al leer.
type xlsxEscenarios struct {
	Items []struct {
		Name    string `xml:"name,attr"`
		Deleted string `xml:"deleted,attr"`
		Inputs  []struct {
			R       string `xml:"r,attr"`
			Val     string `xml:"val,attr"`
			Deleted string `xml:"deleted,attr"`
		} `xml:"inputCells"`
	} `xml:"scenario"`
}

func escenariosDeXlsx(x *xlsxEscenarios) []sheetScenario {
	if x == nil {
		return nil
	}
	var es []sheetScenario
	for _, it := range x.Items {
		if verdadero(it.Deleted) {
			continue
		}
		e := sheetScenario{Name: it.Name, Values: map[string]string{}}
		for _, in := range it.Inputs {
			if !verdadero(in.Deleted) {
				e.Values[strings.ToUpper(strings.TrimSpace(in.R))] = in.Val
			}
		}
		es = append(es, e)
	}
	return sanearEscenarios(es)
}

// escenarioODS es el `table:scenario` de una hoja de escenario de ODF.
type escenarioODS struct {
	rangos string // `table:scenario-ranges`
}

// escenario es el escenario que dice una hoja de escenario: su nombre es el de la hoja y sus
// valores, los de las celdas que cubren sus rangos (en las mismas posiciones que en la hoja base).
func (e *escenarioODS) escenario(nombre string, celdas map[string]string) sheetScenario {
	out := sheetScenario{Name: nombre, Values: map[string]string{}}
	for _, r := range partirRefsODS(e.rangos) {
		rg, ok := refODSRango(r)
		if !ok || !rg.valido() || (rg.r2-rg.r1+1)*(rg.c2-rg.c1+1) > maxValoresEscenario {
			continue
		}
		for f := rg.r1; f <= rg.r2; f++ {
			for c := rg.c1; c <= rg.c2; c++ {
				if v := cellRaw(celdas[A1Ref(f, c)]); v != "" {
					out.Values[A1Ref(f, c)] = v
				}
			}
		}
	}
	return out
}

// hojasEscenarioODS escribe las hojas de escenario de ODF de una hoja, que van justo tras ella:
// una hoja oculta por escenario, con los valores en las mismas celdas que la hoja base.
func hojasEscenarioODS(sd sheetData, usados map[string]bool, styles *odsStyles, b *strings.Builder) {
	for _, e := range sanearEscenarios(sd.Scenarios) {
		nombre := e.Name
		for usados[strings.ToLower(nombre)] {
			nombre += "~"
		}
		usados[strings.ToLower(nombre)] = true
		refs := e.refsOrdenadas()
		rangos := make([]string, len(refs))
		celdas := make(map[string]string, len(refs))
		for i, r := range refs {
			f, c, _ := ParseA1(r)
			rangos[i] = rango{c1: c, r1: f, c2: c, r2: f}.refODS(nombre)
			celdas[r] = html.EscapeString(e.Values[r])
		}
		b.WriteString(`<table:table table:name="` + xmlEscape(nombre) + `"><table:scenario table:display-border="false" table:border-color="#c0c0c0" ` +
			`table:copy-back="false" table:copy-styles="false" table:copy-formulas="false" table:is-active="false" table:scenario-ranges="` +
			xmlEscape(strings.Join(rangos, " ")) + `"/>`)
		odsTable(b, sheetData{Name: nombre, Cells: celdas}, styles, nil, nil)
		b.WriteString(`</table:table>`)
	}
}
