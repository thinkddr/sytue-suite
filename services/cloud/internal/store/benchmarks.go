// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// CollectCohortUsage reduce cada proyecto observado a UNA cifra: su p95 de
// memoria en porcentaje del límite del tamaño que tiene pedido. Cross-tenant, y
// es el único sitio del servicio donde se leen datos de todos los inquilinos a la
// vez para producir algo que verá otro (ver la migración 0029).
//
// Lo que NO sale de aquí es tan importante como lo que sale: ni identificadores
// de proyecto, ni cifras absolutas de memoria. Solo el inquilino —que se usa
// para contar empresas distintas y se descarta al agregar— y el porcentaje.
//
// Un proyecto con menos de MinCohortSamples muestras en la ventana no cuenta: sin
// datos suficientes su p95 no significa nada y sesgaría el consejo de los demás.
func (s *Store) CollectCohortUsage(ctx context.Context, since time.Time) ([]domain.ProjectUsage, error) {
	var out []domain.ProjectUsage
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		// Los servicios se traen como array y la clave la construye
		// domain.ServicesKey, no el SQL: normalizarlo en dos sitios acabaría
		// produciendo cohortes que no casan con la del proyecto que consulta.
		rows, err := tx.Query(ctx, `
			SELECT p.tenant,
			       p.framework,
			       p.instance_size,
			       COALESCE(sv.kinds, ARRAY[]::text[]),
			       percentile_disc(0.95) WITHIN GROUP (ORDER BY rs.memory_bytes)
			FROM projects p
			JOIN resource_samples rs ON rs.project_id = p.id
			LEFT JOIN LATERAL (
				SELECT array_agg(DISTINCT ms.kind) AS kinds
				FROM managed_services ms
				WHERE ms.project_id = p.id AND ms.deleted_at IS NULL
			) sv ON true
			WHERE p.deleted_at IS NULL AND rs.bucket >= $1
			GROUP BY p.id, p.tenant, p.framework, p.instance_size, sv.kinds
			HAVING count(*) >= $2`, since.UTC(), domain.MinCohortSamples)
		if err != nil {
			return fmtErr("recopilar uso por cohorte", err)
		}
		defer rows.Close()
		for rows.Next() {
			var (
				tenant, framework, size string
				kinds                   []string
				p95Bytes                int64
			)
			if err := rows.Scan(&tenant, &framework, &size, &kinds, &p95Bytes); err != nil {
				return err
			}
			svc := make([]domain.ServiceKind, 0, len(kinds))
			for _, k := range kinds {
				svc = append(svc, domain.ServiceKind(k))
			}
			// El tamaño vacío significa «el por defecto»: se resuelve aquí para que
			// todos los proyectos que están en basic caigan en la MISMA cohorte, los
			// haya elegido explícitamente o no.
			size = domain.InstanceSizeOrDefault(size).Name
			out = append(out, domain.ProjectUsage{
				Tenant: tenant,
				Cohort: domain.CohortKey{
					Framework: framework,
					Services:  domain.ServicesKey(svc),
					Size:      size,
				},
				P95MemoryPercent: domain.UsagePercentOfSize(p95Bytes, size),
			})
		}
		return rows.Err()
	})
	return out, err
}

// PublishCohorts sustituye la tabla entera por las cohortes recibidas, en una
// sola transacción.
//
// Reemplazo COMPLETO y no upsert, y esto es una decisión de privacidad, no de
// comodidad: una cohorte que hoy reúne cuatro empresas porque la quinta se dio de
// baja tiene que DESAPARECER. Con upsert se quedaría publicada para siempre con
// las cifras del día que sí llegó al umbral.
func (s *Store) PublishCohorts(ctx context.Context, cohorts []domain.Cohort) error {
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM sizing_benchmarks`); err != nil {
			return fmtErr("vaciar benchmarks", err)
		}
		if len(cohorts) == 0 {
			return nil
		}
		b := &pgx.Batch{}
		for _, c := range cohorts {
			b.Queue(`
				INSERT INTO sizing_benchmarks
					(framework, services, size, p50_memory_percent, p90_memory_percent, tenants, projects, computed_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
				c.Key.Framework, c.Key.Services, c.Key.Size,
				c.P50MemoryPercent, c.P90MemoryPercent, c.Tenants, c.Projects)
		}
		br := tx.SendBatch(ctx, b)
		defer br.Close()
		for range cohorts {
			if _, err := br.Exec(); err != nil {
				// El CHECK (tenants >= 5) de la tabla llega hasta aquí: si el
				// código intentara publicar una cohorte por debajo del umbral,
				// esto falla en vez de dejarla pasar.
				return fmtErr("publicar benchmarks", err)
			}
		}
		return nil
	})
}

// GetCohort devuelve el agregado de una cohorte, o false si no existe (porque no
// hay bastantes empresas comparables, que es el caso normal al principio).
//
// Va bajo el contexto del usuario. La tabla no tiene columna tenant a propósito:
// no hay nada que segmentar, cada fila ya resume varias empresas.
func (s *Store) GetCohort(ctx context.Context, key domain.CohortKey) (domain.Cohort, bool, error) {
	var c domain.Cohort
	var found bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT p50_memory_percent, p90_memory_percent, tenants, projects
			FROM sizing_benchmarks
			WHERE framework = $1 AND services = $2 AND size = $3`,
			key.Framework, key.Services, key.Size,
		).Scan(&c.P50MemoryPercent, &c.P90MemoryPercent, &c.Tenants, &c.Projects)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil
		case err != nil:
			return fmtErr("leer benchmark de cohorte", err)
		}
		c.Key = key
		found = true
		return nil
	})
	return c, found, err
}
