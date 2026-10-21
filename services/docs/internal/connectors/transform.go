// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Transform es la transformación MVP aplicada a la tabla resultante: filtrar
// filas, seleccionar/reordenar columnas y renombrarlas. Es post-proceso puro
// sobre el Result (no toca el origen).
type Transform struct {
	// Select: conservar solo estas columnas, en este orden. Vacío => todas.
	Select []string `json:"select,omitempty"`
	// Rename: mapa nombre original -> nombre nuevo.
	Rename map[string]string `json:"rename,omitempty"`
	// Filter: filtros por columna (AND entre ellos).
	Filter []ColumnFilter `json:"filter,omitempty"`
}

// ColumnFilter es un filtro simple por columna (comparación textual).
type ColumnFilter struct {
	Column string `json:"column"`
	Op     string `json:"op"` // eq | ne | contains
	Value  string `json:"value"`
}

func (t *Transform) validate() error {
	for _, f := range t.Filter {
		if strings.TrimSpace(f.Column) == "" {
			return invalid("filtro: falta la columna")
		}
		switch f.Op {
		case "eq", "ne", "contains":
		default:
			return invalid("filtro: operador inválido %q (eq|ne|contains)", f.Op)
		}
	}
	return nil
}

// applyTransform aplica el transform a un Result: primero filtra filas, luego
// selecciona/reordena columnas y por último renombra. Devuelve un Result nuevo.
//
//nolint:gocognit // tres fases sobre la misma tabla (filtrar, seleccionar, renombrar) con su matriz de operadores.
func applyTransform(res *Result, t *Transform) *Result {
	if t == nil || res == nil {
		return res
	}
	idx := make(map[string]int, len(res.Columns))
	for i, c := range res.Columns {
		idx[c.Name] = i
	}

	rows := res.Rows
	if len(t.Filter) > 0 {
		filtered := make([][]any, 0, len(rows))
		for _, row := range rows {
			if matchesFilters(row, idx, t.Filter) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}

	outCols := res.Columns
	var pick []int // índices de columna a proyectar (nil => todas)
	if len(t.Select) > 0 {
		pick = make([]int, 0, len(t.Select))
		outCols = make([]Column, 0, len(t.Select))
		for _, name := range t.Select {
			if i, ok := idx[name]; ok {
				pick = append(pick, i)
				outCols = append(outCols, res.Columns[i])
			}
		}
	}

	if len(t.Rename) > 0 {
		renamed := make([]Column, len(outCols))
		for i, c := range outCols {
			if nn, ok := t.Rename[c.Name]; ok && strings.TrimSpace(nn) != "" {
				c.Name = nn
			}
			renamed[i] = c
		}
		outCols = renamed
	}

	if pick != nil {
		projected := make([][]any, 0, len(rows))
		for _, row := range rows {
			nr := make([]any, len(pick))
			for j, i := range pick {
				if i < len(row) {
					nr[j] = row[i]
				}
			}
			projected = append(projected, nr)
		}
		rows = projected
	}

	return &Result{Columns: outCols, Rows: rows}
}

// matchesFilters aplica los filtros (AND) a una fila por comparación textual.
func matchesFilters(row []any, idx map[string]int, filters []ColumnFilter) bool {
	for _, f := range filters {
		i, ok := idx[f.Column]
		if !ok || i >= len(row) {
			return false
		}
		s := cellString(row[i])
		switch f.Op {
		case "eq":
			if s != f.Value {
				return false
			}
		case "ne":
			if s == f.Value {
				return false
			}
		case "contains":
			if !strings.Contains(s, f.Value) {
				return false
			}
		}
	}
	return true
}

// cellString reduce un valor de celda a texto para comparar en los filtros.
func cellString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.RawMessage:
		return string(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
