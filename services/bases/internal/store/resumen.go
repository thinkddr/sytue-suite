// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// ResumenVista son las cifras del pie de la rejilla: cuántos registros pasan la vista
// y, por campo, cuántos tienen valor y —en los numéricos— la suma, la media, el mínimo
// y el máximo. Se calculan en Postgres sobre TODO lo que pasa la vista, no sobre la
// ventana cargada: la rejilla enseña cien registros de diez mil, y sumar solo esos
// daría un total que parece cierto y no lo es.
type ResumenVista struct {
	Count  int64                   `json:"count"`
	Fields map[string]ResumenCampo `json:"fields"`
}

// ResumenCampo es el resumen de una columna. Filled cuenta los registros con valor (en
// una casilla, los marcados). Sum, Avg, Min y Max solo vienen en los numéricos con
// algún valor: nil no es cero.
type ResumenCampo struct {
	Filled int64    `json:"filled"`
	Sum    *float64 `json:"sum,omitempty"`
	Avg    *float64 `json:"avg,omitempty"`
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
}

// Resumir calcula el resumen de una vista (o de la tabla entera) con la búsqueda y el
// día de la lectura (lector). UNA consulta: un agregado por columna en la misma pasada,
// con los mismos filtros que la rejilla (`construirConsulta`), así que el pie cuenta
// exactamente lo que la rejilla enseña.
func (s *Store) Resumir(ctx context.Context, a domain.Actor, tabla, vista string, extra LecturaAdHoc) (ResumenVista, error) {
	out := ResumenVista{Fields: map[string]ResumenCampo{}}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		if _, err := autorizarTabla(ctx, tx, a, tabla, domain.AccLeer); err != nil {
			return err
		}
		campos, err := camposDe(ctx, tx, a, tabla)
		if err != nil {
			return err
		}
		cfg, err := configDeLectura(ctx, tx, a, tabla, vista, extra)
		if err != nil {
			return err
		}
		cfg.Sort = nil // el orden no cambia una suma
		q := construirConsulta(campos, cfg)
		sel, lectores := columnasDeResumen(campos, q.add)
		destinos := make([]any, 0, 1+len(lectores)*5)
		destinos = append(destinos, &out.Count)
		for _, l := range lectores {
			destinos = append(destinos, l.destinos()...)
		}
		sql := `SELECT count(*)` + sel + ` FROM base_records r WHERE r.tenant = $1 AND r.table_id = $2::uuid` + q.where
		if err := tx.QueryRow(ctx, sql, append([]any{a.Tenant, tabla}, q.args...)...).Scan(destinos...); err != nil {
			return err
		}
		for _, l := range lectores {
			out.Fields[l.campo] = l.resumen()
		}
		return nil
	})
	return out, err
}

// lectorResumen recoge las columnas de un campo en el SELECT del resumen.
type lectorResumen struct {
	campo              string
	numerico           bool
	filled             int64
	sum, avg, min, max *float64
}

func (l *lectorResumen) destinos() []any {
	if l.numerico {
		return []any{&l.filled, &l.sum, &l.avg, &l.min, &l.max}
	}
	return []any{&l.filled}
}

func (l *lectorResumen) resumen() ResumenCampo {
	return ResumenCampo{Filled: l.filled, Sum: l.sum, Avg: l.avg, Min: l.min, Max: l.max}
}

// columnasDeResumen escribe los agregados de cada campo. Como en los filtros, el id del
// campo va SIEMPRE como parámetro. Un numérico (número, moneda, valoración y fórmula
// de número) suma solo lo que es número en el JSON: una fórmula que da error guarda un
// texto, y convertirlo a numeric tumbaría el resumen entero. Los enlaces cuentan los
// registros con algún enlace; los adjuntos no se resumen.
func columnasDeResumen(campos []domain.Campo, add func(any) string) (string, []*lectorResumen) {
	var b strings.Builder
	lectores := make([]*lectorResumen, 0, len(campos))
	for _, c0 := range campos {
		c := c0.Efectivo()
		l := &lectorResumen{campo: c0.ID}
		switch c.Kind { //nolint:exhaustive // el resto cuenta los que tienen valor
		case domain.TipoAdjunto:
			continue
		case domain.TipoRelacion:
			b.WriteString(`, count(*) FILTER (WHERE EXISTS (SELECT 1 FROM base_links l WHERE l.tenant = r.tenant AND l.field_id = ` + add(c0.ID) + `::uuid AND l.from_record = r.id))`)
		case domain.TipoCasilla:
			b.WriteString(`, count(*) FILTER (WHERE (r.data -> ` + add(c0.ID) + `) = 'true'::jsonb)`)
		case domain.TipoNumero:
			l.numerico = true
			num := `(CASE WHEN jsonb_typeof(r.data -> ` + add(c0.ID) + `) = 'number' THEN (r.data ->> ` + add(c0.ID) + `)::float8 END)`
			b.WriteString(`, count(` + num + `), sum(` + num + `), avg(` + num + `), min(` + num + `), max(` + num + `)`)
		default:
			b.WriteString(`, count(r.data -> ` + add(c0.ID) + `)`)
		}
		lectores = append(lectores, l)
	}
	return b.String(), lectores
}
