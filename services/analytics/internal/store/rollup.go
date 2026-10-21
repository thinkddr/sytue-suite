// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// EnsurePartitions crea las particiones diarias que faltan (desde ayer hasta
// daysAhead por delante). La función es SECURITY DEFINER en la migración porque
// el rol de la plataforma no es dueño de la tabla y no puede hacerle DDL.
func (s *Store) EnsurePartitions(ctx context.Context, daysAhead int) error {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT analytics_ensure_partitions($1)`, daysAhead).Scan(&n); err != nil {
		return fmt.Errorf("analytics: crear particiones: %w", err)
	}
	return nil
}

// ExpirePartitions retira con DROP TABLE las particiones fuera de retención —
// millones de filas sin vacuum ni WAL— y devuelve cuántas. keepDays=2 conserva
// hoy y ayer, de modo que NINGUNA fila de crudo supera nunca las 48 h.
func (s *Store) ExpirePartitions(ctx context.Context, keepDays int) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT analytics_expire_partitions($1)`, keepDays).Scan(&n); err != nil {
		return 0, fmt.Errorf("analytics: purgar particiones: %w", err)
	}
	return n, nil
}

// Rollup recalcula los agregados a partir del crudo. Es idempotente: recalcula
// en vez de acumular, así que repetirlo no falsea nada.
//
// Ventana: las horas de [now-3h, now] y los DÍAS que esa ventana toca. La marca
// de tiempo de un evento la pone el servidor al recibirlo, así que tres horas
// son un margen holgado; en la práctica solo hay dos días en juego durante las
// tres primeras horas de cada jornada.
//
// Los únicos NO se suman: el total diario se calcula sobre el crudo, nunca
// sumando las 24 horas. Por eso hay dos tablas de totales y no una.
func (s *Store) Rollup(ctx context.Context, now time.Time, topN int) error {
	now = now.UTC()
	from := now.Add(-3 * time.Hour).Truncate(time.Hour)
	to := now.Truncate(time.Hour).Add(time.Hour)

	if err := s.rollupHours(ctx, from, to); err != nil {
		return err
	}
	days := []time.Time{dayOf(to.Add(-time.Nanosecond))}
	if d := dayOf(from); !d.Equal(days[0]) {
		days = append([]time.Time{d}, days...)
	}
	for _, day := range days {
		if err := s.rollupDay(ctx, day, topN); err != nil {
			return err
		}
	}
	return nil
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (s *Store) rollupHours(ctx context.Context, from, to time.Time) error {
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO analytics_hourly (tenant, site_id, hour, pageviews, visitors, events)
			SELECT tenant, site_id, date_trunc('hour', ts),
			       count(*) FILTER (WHERE name = 'pageview'),
			       count(DISTINCT visitor),
			       count(*)
			  FROM analytics_events
			 WHERE ts >= $1 AND ts < $2
			 GROUP BY tenant, site_id, date_trunc('hour', ts)
			ON CONFLICT (site_id, hour) DO UPDATE
			   SET tenant = EXCLUDED.tenant,
			       pageviews = EXCLUDED.pageviews,
			       visitors = EXCLUDED.visitors,
			       events = EXCLUDED.events`, from, to)
		return err
	})
	if err != nil {
		return fmt.Errorf("analytics: rollup horario: %w", err)
	}
	return nil
}

// rollupDay recalcula totales y desglose de un día completo.
//
// Las VISITAS se reconstruyen aquí, no en el navegador: mismo visitor, hueco
// menor de 30 minutos. No hay cookie de sesión ni identificador de sesión
// enviado, y la reconstrucción muere con el crudo a las 48 h.
func (s *Store) rollupDay(ctx context.Context, day time.Time, topN int) error {
	next := day.AddDate(0, 0, 1)
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			WITH raw AS (
			    SELECT * FROM analytics_events WHERE ts >= $1 AND ts < $2
			), totals AS (
			    SELECT tenant, site_id,
			           count(*) FILTER (WHERE name = 'pageview') AS pageviews,
			           count(DISTINCT visitor) AS visitors,
			           count(*) AS events
			      FROM raw GROUP BY tenant, site_id
			), seq AS (
			    SELECT site_id, visitor, ts,
			           CASE WHEN lag(ts) OVER w IS NULL
			                  OR ts - lag(ts) OVER w > interval '30 minutes'
			                THEN 1 ELSE 0 END AS starts
			      FROM raw WHERE name = 'pageview'
			    WINDOW w AS (PARTITION BY site_id, visitor ORDER BY ts)
			), grouped AS (
			    SELECT site_id, visitor,
			           sum(starts) OVER (PARTITION BY site_id, visitor ORDER BY ts) AS visit_no
			      FROM seq
			), per_visit AS (
			    SELECT site_id, visitor, visit_no, count(*) AS views
			      FROM grouped GROUP BY site_id, visitor, visit_no
			), visits AS (
			    SELECT site_id, count(*) AS visits,
			           count(*) FILTER (WHERE views = 1) AS bounces
			      FROM per_visit GROUP BY site_id
			)
			INSERT INTO analytics_daily (tenant, site_id, day, pageviews, visitors, visits, bounces, events)
			SELECT t.tenant, t.site_id, $3::date, t.pageviews, t.visitors,
			       COALESCE(v.visits, 0), COALESCE(v.bounces, 0), t.events
			  FROM totals t LEFT JOIN visits v ON v.site_id = t.site_id
			ON CONFLICT (site_id, day) DO UPDATE
			   SET tenant = EXCLUDED.tenant,
			       pageviews = EXCLUDED.pageviews,
			       visitors = EXCLUDED.visitors,
			       visits = EXCLUDED.visits,
			       bounces = EXCLUDED.bounces,
			       events = EXCLUDED.events`, day, next, day); err != nil {
			return fmt.Errorf("totales: %w", err)
		}

		// El desglose se REESCRIBE: si un valor sale del top-N entre dos
		// ejecuciones, su fila vieja tiene que desaparecer, no quedarse mintiendo.
		if _, err := tx.Exec(ctx, `DELETE FROM analytics_daily_breakdown WHERE day = $1::date`, day); err != nil {
			return fmt.Errorf("limpiar desglose: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			WITH raw AS (
			    SELECT * FROM analytics_events WHERE ts >= $1 AND ts < $2
			), dims AS (
			    SELECT tenant, site_id, 'path'      AS dim, path          AS value, visitor FROM raw WHERE name = 'pageview'
			    UNION ALL SELECT tenant, site_id, 'referrer', referrer_host, visitor FROM raw WHERE referrer_host IS NOT NULL
			    UNION ALL SELECT tenant, site_id, 'country',  country,       visitor FROM raw
			    UNION ALL SELECT tenant, site_id, 'browser',  browser,       visitor FROM raw
			    UNION ALL SELECT tenant, site_id, 'os',       os,            visitor FROM raw
			    UNION ALL SELECT tenant, site_id, 'device',   device,        visitor FROM raw
			    UNION ALL SELECT tenant, site_id, 'event',    name,          visitor FROM raw WHERE name <> 'pageview'
			    UNION ALL SELECT tenant, site_id, 'outbound', target,        visitor FROM raw WHERE name = 'outbound' AND target IS NOT NULL
			    UNION ALL SELECT tenant, site_id, 'download', target,        visitor FROM raw WHERE name = 'download' AND target IS NOT NULL
			    UNION ALL SELECT tenant, site_id, 'utm_source',   utm_source,   visitor FROM raw WHERE utm_source IS NOT NULL
			    UNION ALL SELECT tenant, site_id, 'utm_medium',   utm_medium,   visitor FROM raw WHERE utm_medium IS NOT NULL
			    UNION ALL SELECT tenant, site_id, 'utm_campaign', utm_campaign, visitor FROM raw WHERE utm_campaign IS NOT NULL
			    UNION ALL SELECT tenant, site_id, 'utm_content',  utm_content,  visitor FROM raw WHERE utm_content IS NOT NULL
			    UNION ALL SELECT tenant, site_id, 'utm_term',     utm_term,     visitor FROM raw WHERE utm_term IS NOT NULL
			    UNION ALL SELECT tenant, site_id, 'ref',          ref,          visitor FROM raw WHERE ref IS NOT NULL
			), counted AS (
			    SELECT tenant, site_id, dim, value, count(*) AS pageviews
			      FROM dims GROUP BY tenant, site_id, dim, value
			), ranked AS (
			    SELECT *, row_number() OVER (PARTITION BY site_id, dim ORDER BY pageviews DESC, value) AS rn
			      FROM counted
			), labeled AS (
			    SELECT d.tenant, d.site_id, d.dim,
			           CASE WHEN r.rn <= $4 THEN d.value ELSE '(otras)' END AS value,
			           d.visitor
			      FROM dims d
			      JOIN ranked r ON r.site_id = d.site_id AND r.dim = d.dim AND r.value = d.value
			)
			INSERT INTO analytics_daily_breakdown (tenant, site_id, day, dim, value, pageviews, visitors)
			SELECT tenant, site_id, $3::date, dim, value, count(*), count(DISTINCT visitor)
			  FROM labeled GROUP BY tenant, site_id, dim, value`, day, next, day, topN); err != nil {
			return fmt.Errorf("desglose: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("analytics: rollup del día %s: %w", day.Format(time.DateOnly), err)
	}
	return nil
}
