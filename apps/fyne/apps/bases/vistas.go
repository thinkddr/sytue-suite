// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Lo que la interfaz hace con las vistas guardadas: armar la configuración de una vista
// nueva. Espejo de `lib/vistas.ts`. El servicio valida y ejecuta los filtros y el orden
// (no se evalúa nada aquí); esto solo decide cómo se PINTA.

// operadores son los de cada tipo de campo, espejo de `domain.operadores` del servicio.
var operadores = map[string][]string{
	api.BasesKindText:        {"contains", "eq", "neq", "empty", "notempty"},
	api.BasesKindNumber:      {"eq", "neq", "gt", "gte", "lt", "lte", "empty", "notempty"},
	api.BasesKindDate:        {"eq", "neq", "gt", "gte", "lt", "lte", "empty", "notempty"},
	api.BasesKindCheckbox:    {"eq"},
	api.BasesKindSelect:      {"eq", "neq", "empty", "notempty"},
	api.BasesKindMultiselect: {"contains", "empty", "notempty"},
	api.BasesKindLink:        {"empty", "notempty"},
}

// operadoresDe son los operadores de un campo; una fórmula se filtra como su resultado y
// los adjuntos no se filtran.
func operadoresDe(c api.BasesField) []string { return operadores[efectivo(c).Kind] }

// camposDeFormulario son los que admite un formulario público: todos menos enlaces,
// fórmulas y adjuntos.
func camposDeFormulario(campos []api.BasesField) []api.BasesField {
	var out []api.BasesField
	for _, c := range campos {
		if c.Kind != api.BasesKindLink && c.Kind != api.BasesKindFormula && c.Kind != api.BasesKindAttachment {
			out = append(out, c)
		}
	}
	return out
}

// borradorVista es lo que se lleva escrito en el diálogo de «Añadir vista».
type borradorVista struct {
	nombre, tipo string
	// agrupar es el campo de selección de un tablero; campos, los de un formulario.
	agrupar                            string
	campos                             []string
	filtroCampo, filtroOp, filtroValor string
	ordenCampo, ordenDir               string
}

// configDeBorrador es la configuración que se envía al servicio; `ok` falso si el
// borrador no vale todavía.
func configDeBorrador(b borradorVista, campos []api.BasesField) (cfg api.BasesViewConfig, ok bool) {
	if b.tipo == api.BasesKanban {
		if b.agrupar == "" {
			return cfg, false
		}
		cfg.GroupBy = b.agrupar
	}
	if b.tipo == api.BasesForm {
		cfg.Fields = b.campos
		return cfg, len(b.campos) > 0
	}
	if i := slices.IndexFunc(campos, func(c api.BasesField) bool { return c.ID == b.filtroCampo }); i >= 0 {
		f := efectivo(campos[i])
		if !slices.Contains(operadoresDe(f), b.filtroOp) {
			return cfg, false
		}
		filtro := api.BasesFilter{Field: f.ID, Op: b.filtroOp}
		if b.filtroOp != "empty" && b.filtroOp != "notempty" {
			switch f.Kind {
			case api.BasesKindNumber:
				n, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(b.filtroValor), ",", ".", 1), 64)
				if err != nil {
					return cfg, false
				}
				filtro.Value = n
			case api.BasesKindCheckbox:
				filtro.Value = b.filtroValor == "true"
			default:
				if strings.TrimSpace(b.filtroValor) == "" {
					return cfg, false
				}
				filtro.Value = b.filtroValor
			}
		}
		cfg.Filters = []api.BasesFilter{filtro}
	}
	if b.ordenCampo != "" {
		cfg.Sort = []api.BasesSort{{Field: b.ordenCampo, Dir: b.ordenDir}}
	}
	return cfg, true
}

// celda es una posición de la rejilla.
type celda struct{ fila, col int }

// saltoPagina es lo que mueven RePág y AvPág.
const saltoPagina = 10

// moverFoco dice dónde cae el foco tras pulsar `tecla` (patrón «grid» de WAI-ARIA, espejo
// de `lib/rejilla.ts`); `ok` falso si esa tecla no mueve.
func moverFoco(pos celda, tecla string, ctrl bool, filas, cols int) (celda, bool) {
	if filas <= 0 || cols <= 0 {
		return pos, false
	}
	f, c := min(max(pos.fila, 0), filas-1), min(max(pos.col, 0), cols-1)
	switch tecla {
	case "Up":
		return celda{max(f-1, 0), c}, true
	case "Down":
		return celda{min(f+1, filas-1), c}, true
	case "Left":
		return celda{f, max(c-1, 0)}, true
	case "Right":
		return celda{f, min(c+1, cols-1)}, true
	case "Home":
		if ctrl {
			return celda{0, 0}, true
		}
		return celda{f, 0}, true
	case "End":
		if ctrl {
			return celda{filas - 1, cols - 1}, true
		}
		return celda{f, cols - 1}, true
	case "Prior":
		return celda{max(f-saltoPagina, 0), c}, true
	case "Next":
		return celda{min(f+saltoPagina, filas-1), c}, true
	}
	return pos, false
}

// columna es una columna del tablero: el valor de la opción ("" = sin valor) y sus registros.
type columna struct {
	valor     string
	registros []api.BasesRecord
}

// columnasDeTablero reparte los registros en columnas, una por opción del campo de
// selección y, al final, la de «sin valor». Un valor que ya no es una opción (se borró la
// opción) cae en «sin valor» y no desaparece: un registro nunca se pierde de la vista.
func columnasDeTablero(campo api.BasesField, registros []api.BasesRecord) []columna {
	cols := make([]columna, len(campo.Choices)+1)
	for i, o := range campo.Choices {
		cols[i].valor = o
	}
	for _, r := range registros {
		v, _ := r.Data[campo.ID].(string)
		i := slices.Index(campo.Choices, v)
		if i < 0 || v == "" {
			i = len(campo.Choices)
		}
		cols[i].registros = append(cols[i].registros, r)
	}
	return cols
}
