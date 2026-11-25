// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
)

// TipoVista es la forma de mirar una tabla.
type TipoVista string

// Tipos de vista: rejilla, tablero Kanban (por un campo de selección), formulario
// para recoger datos y galería (tarjetas con el primer adjunto de imagen como portada;
// se filtra y se ordena como la rejilla, y la portada la elige quien pinta).
const (
	VistaRejilla    TipoVista = "grid"
	VistaKanban     TipoVista = "kanban"
	VistaFormulario TipoVista = "form"
	VistaGaleria    TipoVista = "gallery"
)

// Filtro y Orden son la configuración de una vista, que NUNCA se concatena en SQL:
// el store solo usa ids de campo ya comprobados contra la tabla y operadores de
// esta lista cerrada.
type Filtro struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value,omitempty"`
}

// Orden de una vista.
type Orden struct {
	Field string `json:"field"`
	Dir   string `json:"dir"` // asc | desc
}

// ConfigVista es el contenido de una vista guardada.
type ConfigVista struct {
	Filters []Filtro `json:"filters,omitempty"`
	Sort    []Orden  `json:"sort,omitempty"`
	GroupBy string   `json:"group_by,omitempty"` // Kanban: campo select
	// Fields: campos visibles del formulario, en orden. Vacío = todos los que no son enlaces.
	Fields []string `json:"fields,omitempty"`
	// Required: los campos del formulario que hay que rellenar para enviarlo. Solo en
	// un formulario, y solo campos que el formulario enseña: obligar a uno que no sale
	// dejaría el formulario imposible de enviar.
	Required []string `json:"required,omitempty"`
	// Search: búsqueda libre de una lectura concreta (`?q=`). Nunca se guarda con la
	// vista: es lo que alguien teclea un momento, no parte de su definición.
	Search string `json:"-"`
	// Today: el día de quien mira (`?today=`), para los filtros de fecha relativos
	// («hoy», «últimos 7 días»). Tampoco se guarda: es de la lectura, y sin él vale el
	// día del servidor, que en UTC va por detrás de Madrid una o dos horas cada noche.
	Today string `json:"-"`
}

// Operadores de fecha relativos a hoy: no comparan con un día fijo sino con el de la
// lectura, así que una vista «Vence esta semana» no hay que editarla cada lunes.
// `past_days` y `next_days` llevan el número de días (1 a MaxDiasRelativos) e
// incluyen hoy; `today` y `this_month` no llevan valor.
const (
	OpHoy         = "today"
	OpEsteMes     = "this_month"
	OpUltimosDias = "past_days"
	OpProximosDia = "next_days"
)

// MaxDiasRelativos es el tope de días de `past_days` y `next_days`: diez años.
const MaxDiasRelativos = 3650

// SinValor dice si un operador no lleva valor.
func SinValor(op string) bool {
	switch op {
	case "empty", "notempty", OpHoy, OpEsteMes:
		return true
	}
	return false
}

// DiasRelativos dice si un operador lleva un número de días como valor.
func DiasRelativos(op string) bool { return op == OpUltimosDias || op == OpProximosDia }

// Vista guardada.
type Vista struct {
	ID        string      `json:"id"`
	TableID   string      `json:"table_id"`
	Name      string      `json:"name"`
	Kind      TipoVista   `json:"kind"`
	Config    ConfigVista `json:"config"`
	CreatedBy string      `json:"created_by"`
}

// Operadores por tipo de campo. Cerrada: añadir uno exige su rama en el store.
var operadores = map[Tipo][]string{
	TipoTexto:     {"eq", "neq", "contains", "empty", "notempty"},
	TipoNumero:    {"eq", "neq", "gt", "gte", "lt", "lte", "empty", "notempty"},
	TipoFecha:     {"eq", "neq", "gt", "gte", "lt", "lte", "empty", "notempty", OpHoy, OpUltimosDias, OpProximosDia, OpEsteMes},
	TipoCasilla:   {"eq"},
	TipoSeleccion: {"eq", "neq", "empty", "notempty"},
	TipoMultiple:  {"contains", "empty", "notempty"},
	TipoRelacion:  {"empty", "notempty"},
}

// ValidarConfig valida una configuración contra los campos de la tabla y
// devuelve la forma canónica. Un filtro sobre un campo que no existe se rechaza.
//
//nolint:gocognit,gocyclo // un bloque por parte de la configuración, todas con el mismo mapa de campos.
func ValidarConfig(kind TipoVista, c ConfigVista, campos []Campo) (ConfigVista, error) {
	if !slices.Contains([]TipoVista{VistaRejilla, VistaKanban, VistaFormulario, VistaGaleria}, kind) {
		return c, ErrInvalido
	}
	porID := make(map[string]Campo, len(campos))
	for _, f := range campos {
		porID[f.ID] = f.Efectivo() // una fórmula se filtra y ordena como su resultado
	}
	if len(c.Filters) > MaxFiltros || len(c.Sort) > MaxOrden || len(c.Fields) > MaxCamposPorTabla {
		return c, fmt.Errorf("%w: vista demasiado grande", ErrInvalido)
	}
	for i, f := range c.Filters {
		campo, ok := porID[f.Field]
		if !ok || !slices.Contains(operadores[campo.Kind], f.Op) {
			return c, fmt.Errorf("%w: filtro %d", ErrInvalido, i+1)
		}
		if SinValor(f.Op) {
			c.Filters[i].Value = nil
			continue
		}
		if DiasRelativos(f.Op) {
			var n float64
			if json.Unmarshal(f.Value, &n) != nil || n != math.Trunc(n) || n < 1 || n > MaxDiasRelativos {
				return c, fmt.Errorf("%w: días del filtro %d", ErrInvalido, i+1)
			}
			c.Filters[i].Value = json.RawMessage(strconv.Itoa(int(n)))
			continue
		}
		// El valor del filtro es del mismo tipo que el campo (contains de texto y de
		// selección múltiple: un texto / una opción).
		ref := campo
		if f.Op == "contains" && campo.Kind == TipoMultiple {
			ref = Campo{Name: campo.Name, Kind: TipoSeleccion, Choices: campo.Choices}
		}
		v, err := ValidarValor(ref, f.Value)
		if err != nil || v == nil {
			return c, fmt.Errorf("%w: valor del filtro %d", ErrInvalido, i+1)
		}
		c.Filters[i].Value = v
	}
	for _, o := range c.Sort {
		campo, ok := porID[o.Field]
		if !ok || campo.Kind == TipoRelacion || campo.Kind == TipoMultiple || campo.Kind == TipoAdjunto || (o.Dir != "asc" && o.Dir != "desc") {
			return c, fmt.Errorf("%w: orden", ErrInvalido)
		}
	}
	if c.GroupBy != "" {
		campo, ok := porID[c.GroupBy]
		if !ok || campo.Kind != TipoSeleccion {
			return c, fmt.Errorf("%w: agrupar solo por un campo de selección", ErrInvalido)
		}
	}
	if kind == VistaKanban && c.GroupBy == "" {
		return c, fmt.Errorf("%w: el tablero necesita un campo de selección", ErrInvalido)
	}
	for _, id := range c.Fields {
		if _, ok := porID[id]; !ok {
			return c, fmt.Errorf("%w: campo del formulario", ErrInvalido)
		}
	}
	if kind != VistaFormulario {
		c.Required = nil
		return c, nil
	}
	// Obligatorio solo lo que el formulario enseña (Fields vacío = todos los que admite)
	// y sin repetir: el formulario público pinta el asterisco de esta lista.
	enFormulario := func(id string) bool {
		// El tipo GUARDADO, no el efectivo: una fórmula se filtra como número, pero no
		// se rellena.
		i := slices.IndexFunc(campos, func(f Campo) bool { return f.ID == id })
		if i < 0 || !AdmiteFormulario(campos[i].Kind) {
			return false
		}
		return len(c.Fields) == 0 || slices.Contains(c.Fields, id)
	}
	vistos := make(map[string]bool, len(c.Required))
	for _, id := range c.Required {
		if !enFormulario(id) || vistos[id] {
			return c, fmt.Errorf("%w: campo obligatorio del formulario", ErrInvalido)
		}
		vistos[id] = true
	}
	return c, nil
}

// AdmiteFormulario dice si un tipo de campo (el guardado) se puede rellenar desde un
// formulario público: enlaces, fórmulas y adjuntos no salen nunca.
func AdmiteFormulario(t Tipo) bool {
	return t != TipoRelacion && t != TipoFormula && t != TipoAdjunto
}
