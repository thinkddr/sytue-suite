// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// TODAS las consultas de este fichero van contra los AGREGADOS. Ninguna toca
// analytics_events: el crudo es donde vive el hash seudónimo, solo existe 48 h y
// el panel no tiene ninguna razón para leerlo. Consecuencia aceptada del ADR:
// las métricas históricas no se pueden redefinir a posteriori.

// Totals son los totales de un rango.
type Totals struct {
	Pageviews int64 `json:"pageviews"`
	// Visitors es la SUMA DE ÚNICOS DIARIOS, no un único de rango deduplicado:
	// eso último exigiría correlacionar entre días y la sal de ayer ya no existe.
	// Quien lo pinte debe etiquetarlo como tal (ADR 0026 §1).
	Visitors int64 `json:"visitors"`
	Visits   int64 `json:"visits"`
	Bounces  int64 `json:"bounces"`
	Events   int64 `json:"events"`
	Days     int64 `json:"days"`
}

// Point es un punto de una serie temporal.
type Point struct {
	At        time.Time `json:"at"`
	Pageviews int64     `json:"pageviews"`
	Visitors  int64     `json:"visitors"`
}

// BreakdownRow es una fila del desglose por dimensión.
type BreakdownRow struct {
	Value     string `json:"value"`
	Pageviews int64  `json:"pageviews"`
	Visitors  int64  `json:"visitors"`
}

// Dimensions son las dimensiones que el rollup materializa. Es allowlist: una
// dimensión que no esté aquí no existe, y así el parámetro del cliente nunca
// llega a decidir qué se consulta.
var Dimensions = []string{
	"path", "referrer", "country", "browser", "os", "device", "event",
	"outbound", "download",
	"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term", "ref",
}

// ValidDimension indica si dim es una dimensión conocida.
func ValidDimension(dim string) bool {
	for _, d := range Dimensions {
		if d == dim {
			return true
		}
	}
	return false
}

// Summary devuelve los totales del rango [from, to] (fechas inclusive).
func (s *Store) Summary(ctx context.Context, siteID string, from, to time.Time) (Totals, error) {
	org, err := organizacion(ctx)
	if err != nil {
		return Totals{}, err
	}
	var t Totals
	err = s.pool.InRLSTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT COALESCE(sum(pageviews), 0), COALESCE(sum(visitors), 0),
			       COALESCE(sum(visits), 0), COALESCE(sum(bounces), 0),
			       COALESCE(sum(events), 0), count(*)
			  FROM analytics_daily
			 WHERE site_id = $1 AND day >= $2::date AND day <= $3::date AND tenant = $4`,
			siteID, from, to, org).
			Scan(&t.Pageviews, &t.Visitors, &t.Visits, &t.Bounces, &t.Events, &t.Days)
	})
	if err != nil {
		return Totals{}, fmt.Errorf("analytics: totales de %s: %w", siteID, err)
	}
	return t, nil
}

// Timeseries devuelve la serie diaria u horaria del rango.
func (s *Store) Timeseries(ctx context.Context, siteID string, from, to time.Time, hourly bool) ([]Point, error) {
	query := `SELECT day::timestamptz, pageviews, visitors FROM analytics_daily
	           WHERE site_id = $1 AND day >= $2::date AND day <= $3::date AND tenant = $4 ORDER BY day`
	if hourly {
		query = `SELECT hour, pageviews, visitors FROM analytics_hourly
		          WHERE site_id = $1 AND hour >= $2::date AND hour < ($3::date + 1) AND tenant = $4 ORDER BY hour`
	}
	org, err := organizacion(ctx)
	if err != nil {
		return nil, err
	}
	var out []Point
	err = s.pool.InRLSTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, siteID, from, to, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p Point
			if err := rows.Scan(&p.At, &p.Pageviews, &p.Visitors); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("analytics: serie de %s: %w", siteID, err)
	}
	return out, nil
}

// Breakdown devuelve el top de una dimensión en el rango. El cajón "(otras)"
// que escribe el rollup viaja como una fila más, con su nombre a la vista.
func (s *Store) Breakdown(ctx context.Context, siteID, dim string, from, to time.Time, limit int) ([]BreakdownRow, error) {
	if !ValidDimension(dim) {
		return nil, fmt.Errorf("analytics: dimensión desconocida %q", dim)
	}
	org, err := organizacion(ctx)
	if err != nil {
		return nil, err
	}
	var out []BreakdownRow
	err = s.pool.InRLSTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT value, sum(pageviews), sum(visitors)
			  FROM analytics_daily_breakdown
			 WHERE site_id = $1 AND dim = $2 AND day >= $3::date AND day <= $4::date AND tenant = $6
			 GROUP BY value
			 ORDER BY sum(pageviews) DESC, value
			 LIMIT $5`, siteID, dim, from, to, limit, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r BreakdownRow
			if err := rows.Scan(&r.Value, &r.Pageviews, &r.Visitors); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("analytics: desglose %s de %s: %w", dim, siteID, err)
	}
	return out, nil
}
