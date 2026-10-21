// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connector

import (
	"encoding/json"
	"errors"
	"sort"
)

// ErrBadQuery señala una consulta tabular inválida por entrada del usuario
// (dataset ausente, spec no-JSON, paginación negativa…). La capa HTTP lo mapea a
// 400; el resto de fallos (destino/upstream) a 502. Los drivers TabularSource lo
// envuelven con %w para dar un mensaje concreto.
var ErrBadQuery = errors.New("connector: consulta inválida")

const (
	// defaultTableRows acota las filas materializadas si el driver no fija un tope.
	defaultTableRows = 10000
	// maxTableColumns protege contra resultados patológicamente anchos.
	maxTableColumns = 512
)

// BuildTable normaliza una lista de registros (objetos JSON ya decodificados) al
// Table del contrato: columnas + filas alineadas. Es el punto ÚNICO de
// materialización tabular de los drivers TabularSource (OData, Odoo…), para no
// duplicar la lógica de unión de columnas/inferencia de tipos en cada uno.
//
// order fija el orden preferente de columnas (p. ej. el $select/fields de la
// consulta); las claves presentes en los registros que no estén en order se
// añaden después en orden alfabético (determinista, sin pérdida de columnas).
// order vacío ⇒ unión de todas las claves en orden alfabético. maxRows<=0 ⇒
// tope por defecto. Los valores compuestos (objeto/array) se serializan a JSON
// crudo; los escalares se dejan tal cual (número/texto/bool/null).
func BuildTable(records []map[string]any, order []string, maxRows int) Table {
	if maxRows <= 0 {
		maxRows = defaultTableRows
	}
	seen := make(map[string]struct{}, len(order))
	names := make([]string, 0, len(order))
	for _, n := range order {
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		names = append(names, n)
	}
	// Claves adicionales presentes en los datos y no listadas en order: alfabético.
	extra := make(map[string]struct{})
	for _, r := range records {
		for k := range r {
			if _, ok := seen[k]; !ok {
				extra[k] = struct{}{}
			}
		}
	}
	ex := make([]string, 0, len(extra))
	for k := range extra {
		ex = append(ex, k)
	}
	sort.Strings(ex)
	names = append(names, ex...)
	if len(names) > maxTableColumns {
		names = names[:maxTableColumns]
	}

	cols := make([]Column, len(names))
	for i, n := range names {
		cols[i] = Column{Name: n, Type: inferColumnType(records, n)}
	}

	rowN := len(records)
	if rowN > maxRows {
		rowN = maxRows
	}
	rows := make([][]any, 0, rowN)
	for _, r := range records[:rowN] {
		row := make([]any, len(names))
		for i, n := range names {
			row[i] = normalizeValue(r[n])
		}
		rows = append(rows, row)
	}
	return Table{Columns: cols, Rows: rows}
}

// inferColumnType deduce el tipo lógico de una columna a partir del primer valor
// no nulo hallado en los registros.
func inferColumnType(records []map[string]any, name string) string {
	for _, r := range records {
		if v, ok := r[name]; ok && v != nil {
			return jsonKind(v)
		}
	}
	return "text"
}

// jsonKind clasifica un valor JSON ya decodificado (encoding/json).
func jsonKind(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64, json.Number:
		return "number"
	case string:
		return "text"
	case []any, map[string]any:
		return "json"
	default:
		return "text"
	}
}

// normalizeValue deja los escalares tal cual y serializa compuestos a JSON crudo.
func normalizeValue(v any) any {
	switch v.(type) {
	case nil, bool, float64, string, json.Number:
		return v
	default: // []any, map[string]any
		if b, err := json.Marshal(v); err == nil {
			return json.RawMessage(b)
		}
		return nil
	}
}
