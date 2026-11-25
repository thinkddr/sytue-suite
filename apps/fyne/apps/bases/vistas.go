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
// nueva y volver a abrir una guardada para editarla. Espejo de `lib/vistas.ts`. El servicio valida y ejecuta los filtros y el orden
// (no se evalúa nada aquí); esto solo decide cómo se PINTA.

// operadores son los de cada tipo de campo, espejo de `domain.operadores` del servicio.
var operadores = map[string][]string{
	api.BasesKindText:        {"contains", "eq", "neq", "empty", "notempty"},
	api.BasesKindNumber:      {"eq", "neq", "gt", "gte", "lt", "lte", "empty", "notempty"},
	api.BasesKindDate:        {"eq", "neq", "gt", "gte", "lt", "lte", "empty", "notempty", "today", "past_days", "next_days", "this_month"},
	api.BasesKindCheckbox:    {"eq"},
	api.BasesKindSelect:      {"eq", "neq", "empty", "notempty"},
	api.BasesKindMultiselect: {"contains", "empty", "notempty"},
	api.BasesKindLink:        {"empty", "notempty"},
}

// opsSinValor son los operadores que no llevan valor (`OPS_SIN_VALOR` de la web): vacío o
// no, y los relativos a un día que pone la lectura («hoy», «este mes»).
var opsSinValor = []string{"empty", "notempty", "today", "this_month"}

// opsConDias son los relativos que llevan un número de días, hoy incluido.
var opsConDias = []string{"past_days", "next_days"}

// maxDias es el tope de días de un filtro relativo (`domain.MaxDiasRelativos`).
const maxDias = 3650

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
	// agrupar es el campo de selección de un tablero; campos, los de un formulario, y
	// obligatorios, los de ellos que hay que rellenar para enviarlo.
	agrupar                            string
	campos                             []string
	obligatorios                       []string
	filtroCampo, filtroOp, filtroValor string
	ordenCampo, ordenDir               string
	// Al editar: los filtros y órdenes que la vista ya tenía después del primero. El
	// diálogo solo enseña uno de cada (la API admite más), y editar una vista no debe
	// perder lo que no se ve; se reenvían tal cual detrás del primero.
	restoFiltros []api.BasesFilter
	restoOrden   []api.BasesSort
}

// borradorDeVista es el borrador de una vista guardada, para abrirla en el diálogo y
// editarla.
func borradorDeVista(v api.BasesView, campos []api.BasesField) borradorVista {
	b := borradorVista{nombre: v.Name, tipo: v.Kind, agrupar: v.Config.GroupBy, filtroOp: "eq", ordenDir: "asc"}
	if b.agrupar == "" {
		if i := slices.IndexFunc(campos, func(c api.BasesField) bool { return efectivo(c).Kind == api.BasesKindSelect }); i >= 0 {
			b.agrupar = campos[i].ID
		}
	}
	b.campos = slices.Clone(v.Config.Fields)
	b.obligatorios = slices.Clone(v.Config.Required)
	// Un formulario sin lista pide todos los que admite: se abre con todos marcados, que
	// es lo mismo que guardarlo así.
	if v.Kind == api.BasesForm && len(b.campos) == 0 {
		for _, c := range camposDeFormulario(campos) {
			b.campos = append(b.campos, c.ID)
		}
	}
	if len(v.Config.Filters) > 0 {
		f := v.Config.Filters[0]
		b.filtroCampo, b.filtroOp = f.Field, f.Op
		switch x := f.Value.(type) {
		case string:
			b.filtroValor = x
		case float64:
			b.filtroValor = strconv.FormatFloat(x, 'f', -1, 64)
		case bool:
			b.filtroValor = strconv.FormatBool(x)
		}
		b.restoFiltros = slices.Clone(v.Config.Filters[1:])
	}
	if len(v.Config.Sort) > 0 {
		b.ordenCampo, b.ordenDir = v.Config.Sort[0].Field, v.Config.Sort[0].Dir
		b.restoOrden = slices.Clone(v.Config.Sort[1:])
	}
	return b
}

// valorDeFiltro es con qué compara un filtro: un número de días en los relativos, nada en
// los que no llevan valor y, en el resto, lo escrito en el tipo del campo. `ok` falso si
// lo escrito no vale.
func valorDeFiltro(f api.BasesField, op, texto string) (valor any, ok bool) {
	switch {
	case slices.Contains(opsConDias, op):
		n, err := strconv.Atoi(strings.TrimSpace(texto))
		return n, err == nil && n >= 1 && n <= maxDias
	case slices.Contains(opsSinValor, op):
		return nil, true
	}
	switch f.Kind {
	case api.BasesKindNumber:
		n, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(texto), ",", ".", 1), 64)
		return n, err == nil
	case api.BasesKindCheckbox:
		return texto == "true", true
	}
	return texto, strings.TrimSpace(texto) != ""
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
		// Obligatorio solo lo que el formulario enseña, en su orden: el servicio rechaza
		// la vista entera por un obligatorio que no sale.
		for _, id := range b.campos {
			if slices.Contains(b.obligatorios, id) {
				cfg.Required = append(cfg.Required, id)
			}
		}
		return cfg, len(b.campos) > 0
	}
	if i := slices.IndexFunc(campos, func(c api.BasesField) bool { return c.ID == b.filtroCampo }); i >= 0 {
		f := efectivo(campos[i])
		if !slices.Contains(operadoresDe(f), b.filtroOp) {
			return cfg, false
		}
		valor, vale := valorDeFiltro(f, b.filtroOp, b.filtroValor)
		if !vale {
			return cfg, false
		}
		cfg.Filters = []api.BasesFilter{{Field: f.ID, Op: b.filtroOp, Value: valor}}
	}
	// Lo que se conserva de la vista editada, salvo lo de campos que ya no existen: el
	// servicio rechazaría la vista entera por un filtro sobre un campo borrado.
	existe := func(id string) bool {
		return slices.ContainsFunc(campos, func(c api.BasesField) bool { return c.ID == id })
	}
	for _, x := range b.restoFiltros {
		if existe(x.Field) {
			cfg.Filters = append(cfg.Filters, x)
		}
	}
	if b.ordenCampo != "" {
		cfg.Sort = []api.BasesSort{{Field: b.ordenCampo, Dir: b.ordenDir}}
	}
	for _, x := range b.restoOrden {
		if existe(x.Field) {
			cfg.Sort = append(cfg.Sort, x)
		}
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
