// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"fmt"
	"slices"
)

// TipoVista es la forma de mirar una tabla.
type TipoVista string

// Tipos de vista: rejilla, tablero Kanban (por un campo de selección) y formulario
// para recoger datos.
const (
	VistaRejilla    TipoVista = "grid"
	VistaKanban     TipoVista = "kanban"
	VistaFormulario TipoVista = "form"
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
}

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
	TipoFecha:     {"eq", "neq", "gt", "gte", "lt", "lte", "empty", "notempty"},
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
	if kind != VistaRejilla && kind != VistaKanban && kind != VistaFormulario {
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
		if f.Op == "empty" || f.Op == "notempty" {
			c.Filters[i].Value = nil
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
	return c, nil
}
