// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"encoding/json"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// consulta es el WHERE y el ORDER BY de una vista, con sus parámetros.
type consulta struct {
	where string // empieza por " AND " o vacío
	order string
	args  []any // desde $3 (el $1 es el tenant y el $2 la tabla)
	// claves son las expresiones del ORDER BY que salen de la vista, en orden: lo que
	// el cursor de página necesita para saber dónde se quedó (cursor.go).
	claves []claveOrden
}

// claveOrden es una clave del orden de una vista.
type claveOrden struct {
	campo string
	tipo  domain.Tipo
	expr  string // SQL, con el id del campo como parámetro
	desc  bool
}

// add enlaza un parámetro y devuelve su marcador.
func (q *consulta) add(v any) string {
	q.args = append(q.args, v)
	return "$" + strconv.Itoa(len(q.args)+2) + "::text"
}

// construirConsulta traduce la configuración de una vista a SQL SIN concatenar
// nada que venga del cliente: los ids de campo salen de los campos reales de la
// tabla y viajan como parámetros (`r.data ->> $n`), los operadores son una lista
// cerrada y la dirección del orden es una de dos constantes. Un filtro sobre un
// campo que ya no existe (borrado después de guardar la vista) se ignora.
//
//nolint:gocognit,gocyclo // un caso por operador y tipo; es la única función que escribe SQL de filtros.
func construirConsulta(campos []domain.Campo, cfg domain.ConfigVista) consulta {
	porID := make(map[string]domain.Campo, len(campos))
	for _, c := range campos {
		porID[c.ID] = c.Efectivo() // una fórmula se filtra y ordena como su resultado
	}
	q := consulta{}
	add := q.add
	var where []string
	for _, f := range cfg.Filters {
		c, ok := porID[f.Field]
		if !ok {
			continue
		}
		if c.Kind == domain.TipoRelacion {
			ex := `EXISTS (SELECT 1 FROM base_links l WHERE l.tenant = r.tenant AND l.field_id = ` + add(c.ID) + `::uuid AND l.from_record = r.id)`
			switch f.Op {
			case "notempty":
				where = append(where, ex)
			case "empty":
				where = append(where, "NOT "+ex)
			}
			continue
		}
		// Cada parámetro se enlaza justo donde se usa: uno sin referenciar es un error.
		col := func() string { return "(r.data ->> " + add(c.ID) + ")" }
		switch f.Op {
		case "empty":
			where = append(where, col()+" IS NULL")
			continue
		case "notempty":
			where = append(where, col()+" IS NOT NULL")
			continue
		}
		if c.Kind == domain.TipoFecha {
			if w, ok := condicionRelativa(f, col, hoySQL(cfg.Today, add)); ok {
				where = append(where, w)
				continue
			}
		}
		v, ok := valorFiltro(c, f)
		if !ok {
			continue
		}
		switch {
		case c.Kind == domain.TipoMultiple && f.Op == "contains":
			where = append(where, "(r.data -> "+add(c.ID)+") @> to_jsonb("+add(v)+")")
		case c.Kind == domain.TipoCasilla:
			where = append(where, "coalesce("+col()+"::boolean, false) = "+add(v)+"::boolean")
		case f.Op == "contains":
			// position() y no LIKE: los comodines del usuario no significan nada.
			where = append(where, "position(lower("+add(v)+") in lower(coalesce("+col()+", ''))) > 0")
		case f.Op == "eq" && c.Kind == domain.TipoNumero:
			where = append(where, col()+"::numeric = "+add(v)+"::numeric")
		case f.Op == "neq" && c.Kind == domain.TipoNumero:
			where = append(where, col()+"::numeric IS DISTINCT FROM "+add(v)+"::numeric")
		case f.Op == "eq":
			where = append(where, col()+" = "+add(v))
		case f.Op == "neq":
			where = append(where, col()+" IS DISTINCT FROM "+add(v))
		default: // gt gte lt lte
			op := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[f.Op]
			if op == "" {
				continue
			}
			if c.Kind == domain.TipoNumero {
				where = append(where, col()+"::numeric "+op+" "+add(v)+"::numeric")
			} else { // fechas ISO: el orden de texto es el cronológico
				where = append(where, col()+" "+op+" "+add(v))
			}
		}
	}
	if b := strings.TrimSpace(cfg.Search); b != "" {
		where = append(where, condicionBusqueda(campos, porID, b, add))
	}
	if len(where) > 0 {
		q.where = " AND " + strings.Join(where, " AND ")
	}
	var orden []string
	for _, o := range cfg.Sort {
		c, ok := porID[o.Field]
		if !ok {
			continue
		}
		e := "(r.data ->> " + add(c.ID) + ")"
		switch c.Kind { //nolint:exhaustive // el resto se ordena como texto
		case domain.TipoNumero:
			e += "::numeric"
		case domain.TipoCasilla:
			e += "::boolean"
		default:
			// Un texto ordena por su principio: así la clave cabe en el cursor de página
			// (un texto admite 10.000 caracteres y el cursor viaja en la URL). Los que
			// empatan en ese principio se desempatan por antigüedad, como todo.
			e = "left" + e[:len(e)-1] + ", " + strconv.Itoa(domain.MaxClaveOrden) + ")"
		}
		dir := " ASC"
		if o.Dir == "desc" {
			dir = " DESC"
		}
		orden = append(orden, e+dir+" NULLS LAST")
		q.claves = append(q.claves, claveOrden{campo: c.ID, tipo: c.Kind, expr: e, desc: o.Dir == "desc"})
	}
	orden = append(orden, "r.created_at", "r.id")
	q.order = strings.Join(orden, ", ")
	return q
}

// hoySQL es la expresión del día de la lectura: el de quien mira si lo dijo (`?today=`,
// ya validado como AAAA-MM-DD), o el del servidor. Va como parámetro, nunca pegado.
func hoySQL(hoy string, add func(any) string) func() string {
	return func() string {
		if hoy == "" {
			return "current_date"
		}
		return add(hoy) + "::date"
	}
}

// condicionRelativa traduce un filtro de fecha relativo a hoy. Las fechas se guardan
// como AAAA-MM-DD, así que se compara texto con texto (el orden de texto es el
// cronológico) y la columna no se convierte: así no falla nunca por un valor raro.
// `past_days` y `next_days` incluyen hoy: «últimos 7 días» son hoy y los seis de antes.
func condicionRelativa(f domain.Filtro, col, hoy func() string) (string, bool) {
	dia := func(expr string) string { return "to_char(" + expr + ", 'YYYY-MM-DD')" }
	switch f.Op {
	case domain.OpHoy:
		return col() + " = " + dia(hoy()), true
	case domain.OpEsteMes:
		return "(" + col() + " >= " + dia("date_trunc('month', "+hoy()+")") +
			" AND " + col() + " < " + dia("date_trunc('month', "+hoy()+") + interval '1 month'") + ")", true
	case domain.OpUltimosDias, domain.OpProximosDia:
		var n int
		if json.Unmarshal(f.Value, &n) != nil || n < 1 || n > domain.MaxDiasRelativos {
			return "false", true // un filtro roto no deja pasar todo
		}
		dias := strconv.Itoa(n - 1) // entero ya acotado: no viene texto del cliente
		desde, hasta := dia(hoy()+" - "+dias), dia(hoy())
		if f.Op == domain.OpProximosDia {
			desde, hasta = dia(hoy()), dia(hoy()+" + "+dias)
		}
		return "(" + col() + " BETWEEN " + desde + " AND " + hasta + ")", true
	}
	return "", false
}

// condicionBusqueda: el registro contiene el texto (sin distinguir mayúsculas) en alguno
// de sus campos con valor escrito. Los enlaces y adjuntos no cuentan (guardan ids, no lo
// que se ve) y una casilla no tiene texto que buscar. Sin campos buscables, nada casa:
// buscar algo nunca devuelve «todo».
func condicionBusqueda(campos []domain.Campo, porID map[string]domain.Campo, texto string, add func(any) string) string {
	o := make([]string, 0, len(campos))
	for _, c0 := range campos {
		c := porID[c0.ID]
		switch c.Kind { //nolint:exhaustive // el resto lleva un valor con texto
		case domain.TipoRelacion, domain.TipoAdjunto, domain.TipoCasilla:
			continue
		}
		o = append(o, "position(lower("+add(texto)+") in lower(coalesce(r.data ->> "+add(c.ID)+", ''))) > 0")
	}
	if len(o) == 0 {
		return "false"
	}
	return "(" + strings.Join(o, " OR ") + ")"
}

// valorFiltro pasa el valor de un filtro a texto para el parámetro.
func valorFiltro(c domain.Campo, f domain.Filtro) (string, bool) {
	switch c.Kind { //nolint:exhaustive // el resto de tipos lleva un texto
	case domain.TipoNumero:
		var n float64
		if json.Unmarshal(f.Value, &n) != nil {
			return "", false
		}
		return strconv.FormatFloat(n, 'f', -1, 64), true
	case domain.TipoCasilla:
		var b bool
		if json.Unmarshal(f.Value, &b) != nil {
			return "", false
		}
		return strconv.FormatBool(b), true
	}
	var s string
	if json.Unmarshal(f.Value, &s) != nil {
		return "", false
	}
	return s, true
}
