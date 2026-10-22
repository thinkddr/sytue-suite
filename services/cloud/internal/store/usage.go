// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// UsageInput es un dato de uso a registrar (facturación).
type UsageInput struct {
	Tenant     string
	ProjectID  *uuid.UUID
	Metric     string
	Quantity   float64
	Unit       string
	PeriodFrom time.Time
	PeriodTo   time.Time
}

// RecordUsage inserta un registro de uso. Cross-tenant: lo emiten los workers
// (builder/controller) con el tenant correspondiente.
func (s *Store) RecordUsage(ctx context.Context, in UsageInput) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, in.Tenant); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO usage_records (tenant, project_id, metric, quantity, unit, period_from, period_to)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			in.Tenant, in.ProjectID, in.Metric, in.Quantity, in.Unit, in.PeriodFrom, in.PeriodTo)
		if err != nil {
			return fmtErr("registrar uso", err)
		}
		return nil
	})
}

// ComputeUsage es una ventana de cómputo reclamada de una producción ready: el
// reconciliador la traduce a un registro compute_gb_hours vía RecordUsage.
type ComputeUsage struct {
	Tenant    string
	ProjectID uuid.UUID
	Replicas  int // réplicas efectivas (>= 1)
	// Size es el nombre del tamaño de instancia del proyecto: de él sale la
	// memoria facturable. Antes se asumía un valor fijo para todas las apps.
	Size  string
	Since time.Time // inicio de la ventana medida [Since, now)
}

// ClaimComputeUsage reclama, en UNA sola pasada, las ventanas de cómputo de las
// producciones ready cuya última medición (metered_at, o ready_at/created_at si
// nunca se midió) es más antigua que minWindow, avanza su metered_at a `now`
// atómicamente y devuelve la ventana [Since, now) de cada una. Since se acota a
// now-maxWindow (clamp) para no facturar retroactivamente ni una producción medida
// por primera vez (metered_at NULL con ready_at antiguo) ni el hueco de una caída
// larga del controller. Cross-tenant (reconciliador) vía pg.InMaintenanceTx, que
// habilita las políticas de mantenimiento de deployments y projects (0002/0003);
// con FORCE RLS sería deny-by-default. FOR UPDATE ... SKIP LOCKED evita doble cobro
// entre controllers concurrentes; el LIMIT pagina el backlog (FIFO por antigüedad
// de medición) y se drena en varios ciclos. Idempotente: cada [metered_at, now) se
// factura una sola vez porque metered_at avanza en la misma sentencia que lee Since.
func (s *Store) ClaimComputeUsage(ctx context.Context, now time.Time, minWindow, maxWindow time.Duration, limit int) ([]ComputeUsage, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	cutoff := now.Add(-minWindow) // vencidas: última medición <= cutoff
	floor := now.Add(-maxWindow)  // suelo de la ventana (clamp)
	var out []ComputeUsage
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH due AS (
			    SELECT d.id,
			           d.tenant,
			           d.project_id,
			           -- Escala y tamaño salen del PROYECTO, que es donde vive la
			           -- configuración vigente. La columna replicas del deployment es
			           -- el valor con el que se creó y queda obsoleta en cuanto se
			           -- escala, así que facturar por ella cobraría de menos a quien
			           -- acaba de subir instancias.
			           GREATEST(p.instance_count, 1) AS replicas,
			           p.instance_size AS size,
			           GREATEST(COALESCE(d.metered_at, d.ready_at, d.created_at), $3) AS since
			    FROM deployments d
			    JOIN projects p ON p.id = d.project_id AND p.tenant = d.tenant
			    WHERE d.target = 'production' AND d.status = 'ready'
			      AND p.deleted_at IS NULL
			      -- Pausado = apagado: cero réplicas en el clúster y cero factura.
			      -- Si esta línea faltara, el ahorro que promete el botón sería
			      -- mentira y nadie lo notaría hasta ver el recibo.
			      AND NOT p.paused
			      AND COALESCE(d.metered_at, d.ready_at, d.created_at) <= $2
			    ORDER BY COALESCE(d.metered_at, d.ready_at, d.created_at)
			    FOR UPDATE OF d SKIP LOCKED
			    LIMIT $4
			), upd AS (
			    UPDATE deployments d SET metered_at = $1 FROM due WHERE d.id = due.id
			)
			SELECT tenant, project_id, replicas, size, since FROM due`,
			now, cutoff, floor, limit)
		if err != nil {
			return fmtErr("reclamar uso de cómputo", err)
		}
		defer rows.Close()
		for rows.Next() {
			var u ComputeUsage
			if err := rows.Scan(&u.Tenant, &u.ProjectID, &u.Replicas, &u.Size, &u.Since); err != nil {
				return err
			}
			out = append(out, u)
		}
		return rows.Err()
	})
	return out, err
}

// UsageAggregate es el total de una métrica de uso en el periodo consultado.
type UsageAggregate struct {
	Metric string  `json:"metric"`
	Total  float64 `json:"total"`
	Unit   string  `json:"unit"`
}

// SummarizeUsage agrega el uso del tenant por métrica en [from, to). Cada
// registro se atribuye al periodo por el INICIO de su ventana (period_from): así
// una ventana que cruza el límite de mes (p. ej. [30-jun 23:30, 1-jul 00:30)) cae
// exactamente en un periodo y no se pierde de ambos (evita fuga de ingresos en el
// cambio de mes) ni se duplica entre meses adyacentes.
func (s *Store) SummarizeUsage(ctx context.Context, from, to time.Time) ([]UsageAggregate, error) {
	t := tenancy.MustFrom(ctx)
	var out []UsageAggregate
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT metric, sum(quantity) AS total, coalesce(max(unit), '') AS unit
			FROM usage_records
			WHERE tenant=$1 AND period_from >= $2 AND period_from < $3
			GROUP BY metric ORDER BY metric`, t.Slug, from, to)
		if err != nil {
			return fmtErr("agregar uso", err)
		}
		defer rows.Close()
		for rows.Next() {
			var a UsageAggregate
			if err := rows.Scan(&a.Metric, &a.Total, &a.Unit); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// ListUsage lista registros de uso del tenant en un periodo (detalle).
func (s *Store) ListUsage(ctx context.Context, from, to time.Time, limit int) ([]domain.UsageRecord, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var out []domain.UsageRecord
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, project_id, metric, quantity, unit, period_from, period_to, created_at
			FROM usage_records
			WHERE tenant=$1 AND period_from >= $2 AND period_from < $3
			ORDER BY period_from DESC LIMIT $4`, t.Slug, from, to, limit)
		if err != nil {
			return fmtErr("listar uso", err)
		}
		defer rows.Close()
		for rows.Next() {
			var u domain.UsageRecord
			if err := rows.Scan(&u.ID, &u.Tenant, &u.ProjectID, &u.Metric, &u.Quantity,
				&u.Unit, &u.PeriodFrom, &u.PeriodTo, &u.CreatedAt); err != nil {
				return err
			}
			u.Tenant = t.Slug
			out = append(out, u)
		}
		return rows.Err()
	})
	return out, err
}
