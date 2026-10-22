// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/xml"
	"math"
	"slices"
	"strconv"
	"strings"
)

// El AUTOFILTRO de una hoja (sheet_filtro.go) en ODF: un `table:database-range` anónimo con
// los botones de filtro y un `table:filter` de condiciones unidas por «y». ODF (y Calc) solo
// sabe una operación para todo el filtro, así que las condiciones que Excel resuelve con «o»
// por columna (fuera de un intervalo) o que no tienen operador (por encima de la media, por
// color) no se escriben. Una lista de valores excluidos son tantas condiciones «distinto de»
// como excluidos.

// condicionODS es un `table:filter-condition`.
type condicionODS struct {
	campo  int // relativo al rango, base 0
	op     string
	valor  string
	numero bool
	// set son los valores que SE VEN de una lista (`table:filter-set-item`); conSet dice que
	// la condición es una lista, aunque no se vea ninguno.
	set    []string
	conSet bool
}

func (c condicionODS) xml() string {
	x := `<table:filter-condition table:field-number="` + strconv.Itoa(c.campo) + `" table:value="` + xmlEscape(c.valor) +
		`" table:operator="` + xmlEscape(c.op) + `"`
	if c.numero {
		x += ` table:data-type="number"`
	}
	if !c.conSet {
		return x + `/>`
	}
	x += `>`
	for _, v := range c.set {
		x += `<table:filter-set-item table:value="` + xmlEscape(v) + `"/>`
	}
	return x + `</table:filter-condition>`
}

var operadorFiltroODS = map[string]string{
	"gt": ">", "ge": ">=", "lt": "<", "le": "<=", "eq": "=", "ne": "!=",
	"contains": "contains", "notContains": "!contains", "begins": "begins", "ends": "ends",
}

// operandoFiltroODS es un operando del editor como valor de una condición: un número en
// notación plana o el texto (Calc decide por sí mismo si el valor es un número).
func operandoFiltroODS(s string) string {
	if n, ok := parseNumES(s); ok {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return strings.TrimSpace(s)
}

// condicionesDeColumnaODS son las condiciones de una columna del filtro (campo relativo).
func condicionesDeColumnaODS(af *sheetAutoFilter, fc *filtroCalculado, col, campo int) []condicionODS {
	clave := strconv.Itoa(col)

	if len(af.Excl[clave]) > 0 {
		// Como Calc: una condición «igual a» con la lista de lo que SE VE.
		visibles := fc.visibles[col]
		if len(visibles) == 0 && !fc.vacioVisible[col] {
			return []condicionODS{{campo: campo, op: "empty"}, {campo: campo, op: "!empty"}} // todo oculto
		}
		set := append([]string(nil), visibles...)
		if fc.vacioVisible[col] {
			set = append(set, "")
		}
		primero := ""
		if len(set) > 0 {
			primero = set[0]
		}
		return []condicionODS{{campo: campo, op: "=", valor: primero, set: set, conSet: true}}
	}
	cond, ok := af.Cond[clave]
	if !ok {
		return nil
	}
	a, b := operandoFiltroODS(cond.A), operandoFiltroODS(cond.B)
	switch cond.Op {
	case "gt", "ge", "lt", "le", "eq", "ne":
		return []condicionODS{{campo: campo, op: operadorFiltroODS[cond.Op], valor: a}}
	case "contains", "notContains", "begins", "ends":
		return []condicionODS{{campo: campo, op: operadorFiltroODS[cond.Op], valor: strings.TrimSpace(cond.A)}}
	case "between":
		return []condicionODS{{campo: campo, op: ">=", valor: a}, {campo: campo, op: "<=", valor: b}}
	case "blank":
		return []condicionODS{{campo: campo, op: "empty"}}
	case "notBlank":
		return []condicionODS{{campo: campo, op: "!empty"}}
	case "top", "bottom":
		u := "values"
		if cond.Percent {
			u = "percent"
		}
		return []condicionODS{{campo: campo, op: cond.Op + " " + u, valor: strconv.Itoa(cond.N)}}
	}
	return nil
}

// rangoFiltroODS es el `table:database-range` anónimo del autofiltro de la hoja; "" si no
// hay o si cae sobre una tabla (ahí lo lleva la propia tabla).
func rangoFiltroODS(sd sheetData, hoja string, n int) string {
	af, fc := sd.AutoFilter, sd.filtro
	if af == nil || fc == nil {
		return ""
	}
	for _, t := range sd.Tables {
		if r, ok := leerRango(t.Range); ok && r.r1 <= fc.ref.r2 && r.r2 >= fc.ref.r1 && r.c1 <= fc.ref.c2 && r.c2 >= fc.ref.c1 {
			return ""
		}
	}
	var conds strings.Builder
	for c := af.C1; c <= af.C2; c++ {
		for _, x := range condicionesDeColumnaODS(af, fc, c, c-af.C1) {
			conds.WriteString(x.xml())
		}
	}
	x := `<table:database-range table:name="__Anonymous_Sheet_DB__` + strconv.Itoa(n) + `" table:target-range-address="` +
		xmlEscape(fc.ref.refODS(hoja)) + `" table:display-filter-buttons="true">`
	if conds.Len() > 0 {
		x += `<table:filter><table:filter-and>` + conds.String() + `</table:filter-and></table:filter>`
	}
	return x + `</table:database-range>`
}

// --- Importar ---

// filtroODS es un `database-range` con botones de filtro: su rango (con el nombre de la hoja)
// y las condiciones por campo.
type filtroODS struct {
	hoja    string
	rango   rango
	conds   map[int][]condicionODS
	botones bool
	anonimo bool
}

// leerFiltroODS lee un `table:database-range` (ya abierto) con sus condiciones, hasta su cierre.
func leerFiltroODS(dec *xml.Decoder, se xml.StartElement) (filtroODS, bool) {
	f := filtroODS{
		conds: map[int][]condicionODS{}, botones: attrLocal(se, "display-filter-buttons") == "true",
		anonimo: strings.HasPrefix(attrLocal(se, "name"), "__Anonymous"),
	}
	destino := attrLocal(se, "target-range-address")
	f.hoja = destino
	if i := strings.Index(destino, "."); i >= 0 {
		f.hoja = strings.ReplaceAll(strings.Trim(destino[:i], "'$"), "''", "'")
	}
	r, ok := refODSRango(destino)
	f.rango = r
	ultimo := -1
	for prof := 1; prof > 0; {
		tok, err := dec.Token()
		if err != nil {
			return f, false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			prof++
			if t.Name.Local == "filter-set-item" && ultimo >= 0 && len(f.conds[ultimo]) > 0 {
				c := &f.conds[ultimo][len(f.conds[ultimo])-1]
				c.set = append(c.set, attrLocal(t, "value"))
			}
			if t.Name.Local == "filter-condition" {
				if campo, err := strconv.Atoi(attrLocal(t, "field-number")); err == nil && campo >= 0 && campo < maxColumnasHoja {
					f.conds[campo] = append(f.conds[campo], condicionODS{
						campo: campo, op: attrLocal(t, "operator"), valor: attrLocal(t, "value"),
						numero: attrLocal(t, "data-type") == "number",
					})
					ultimo = campo
				}
			}
		case xml.EndElement:
			prof--
		}
	}
	return f, ok && f.rango.valido()
}

// valorCondicionODS es el valor de una condición como texto del editor: un número con coma
// decimal, o el texto tal cual.
func valorCondicionODS(c condicionODS) string {
	if n, err := strconv.ParseFloat(strings.TrimSpace(c.valor), 64); err == nil && !math.IsInf(n, 0) && !math.IsNaN(n) {
		return textoNumEs(n)
	}
	return c.valor
}

// autoFilterDeODS pasa un `database-range` con filtro al autofiltro del editor; nil si no es
// un autofiltro de hoja. `cells` son las de su hoja: una lista de lo que se ve se convierte
// en los valores EXCLUIDOS de la columna.
func autoFilterDeODS(f filtroODS, cells map[string]string) *sheetAutoFilter {
	if !f.botones || !f.anonimo {
		return nil
	}
	af := &sheetAutoFilter{Row: f.rango.r1 - 1, C1: f.rango.c1 - 1, C2: f.rango.c2 - 1}
	for campo, cs := range f.conds {
		col := f.rango.c1 - 1 + campo
		if col > af.C2 {
			continue
		}
		clave := strconv.Itoa(col)
		if len(cs) == 1 && len(cs[0].set) > 0 {
			if excl := excluidosDeLista(cs[0].set, f.rango, col, cells); len(excl) > 0 {
				ponerExcl(af, clave, excl)
			}
			continue
		}
		if cond, ok := condicionDeODS(cs); ok {
			if cond.Op == "excl" {
				for _, c := range cs {
					ponerExcl(af, clave, []string{valorCondicionODS(c)})
				}
				continue
			}
			if af.Cond == nil {
				af.Cond = map[string]filterCond{}
			}
			af.Cond[clave] = cond
		}
	}
	return af
}

// condicionDeODS traduce las condiciones de UN campo; Op «excl» marca una lista de «distinto
// de» (los valores se leen de las propias condiciones). false si no tiene equivalente.
func condicionDeODS(cs []condicionODS) (filterCond, bool) {
	cmp := map[string]string{
		">": "gt", ">=": "ge", "<": "lt", "<=": "le", "=": "eq", "contains": "contains",
		"!contains": "notContains", "begins": "begins", "ends": "ends",
	}
	if len(cs) == 1 {
		c := cs[0]
		switch c.op {
		case "empty":
			return filterCond{Op: "blank"}, true
		case "!empty":
			return filterCond{Op: "notBlank"}, true
		case "!=":
			return filterCond{Op: "excl"}, true
		case "top values", "bottom values", "top percent", "bottom percent":
			n, err := strconv.Atoi(strings.TrimSpace(c.valor))
			if err != nil || n < 1 {
				return filterCond{}, false
			}
			return filterCond{Op: strings.SplitN(c.op, " ", 2)[0], N: n, Percent: strings.HasSuffix(c.op, "percent")}, true
		}
		if o, ok := cmp[c.op]; ok {
			return filterCond{Op: o, A: valorCondicionODS(c)}, true
		}
		return filterCond{}, false
	}
	todosDistintos := true
	for _, c := range cs {
		todosDistintos = todosDistintos && (c.op == "!=" || c.op == "!empty")
	}
	if todosDistintos {
		return filterCond{Op: "excl"}, true
	}
	if len(cs) == 2 && cs[0].op == ">=" && cs[1].op == "<=" {
		return filterCond{Op: "between", A: valorCondicionODS(cs[0]), B: valorCondicionODS(cs[1])}, true
	}
	return filterCond{}, false
}

// filtradaODS es el ` table:visibility="filter"` de una fila que el autofiltro esconde
// (si ya la oculta un grupo colapsado, manda ese).
func filtradaODS(sd sheetData, fila int) string {
	if sd.filtro != nil && sd.filtro.ocultas[fila] {
		if _, oculta := nivelDe(sd.RowGroups, fila-1); !oculta {
			return ` table:visibility="filter"`
		}
	}
	return ""
}

// excluidosDeLista son los valores literales de la columna que una lista de «lo que se ve»
// no deja ver; el valor de una fórmula no se conoce al importar y no cuenta.
func excluidosDeLista(visibles []string, ref rango, col int, cells map[string]string) []string {
	var excl []string
	for r := ref.r1 + 1; r <= ref.r2; r++ {
		t := cellRaw(cells[A1Ref(r, col+1)])
		if strings.HasPrefix(t, "=") || slices.Contains(visibles, t) || slices.Contains(excl, t) {
			continue
		}
		excl = append(excl, t)
	}
	return excl
}

func ponerExcl(af *sheetAutoFilter, clave string, valores []string) {
	if af.Excl == nil {
		af.Excl = map[string][]string{}
	}
	af.Excl[clave] = append(af.Excl[clave], valores...)
}
