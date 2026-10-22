// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// SampleBucket es el tamaño del cubo de agregación del muestreo (ver la
// migración 0026). Vive aquí porque el SQL de RecordResourceSamples lo trunca y
// el muestreador tiene que espaciarse igual: dos verdades separadas producirían
// cubos a medio llenar sin que nada fallara.
const SampleBucket = 15 * time.Minute

// ResourceSampleInput es una observación del uso de UN proyecto: el máximo entre
// sus pods de producción en el instante del muestreo.
type ResourceSampleInput struct {
	Tenant      string
	ProjectID   uuid.UUID
	At          time.Time
	CPUMilli    int
	MemoryBytes int64
}

// RecordResourceSamples persiste un lote de observaciones, cross-tenant.
//
// Idempotente y seguro con varias réplicas del controlador: la clave primaria es
// (project_id, cubo) y el conflicto se resuelve con GREATEST, no con el último
// que escriba. Eso conserva el PICO del intervalo, que es exactamente el dato del
// que depende no recomendar una bajada que acabe en OOMKill — quedarse con la
// última muestra lo perdería en silencio.
func (s *Store) RecordResourceSamples(ctx context.Context, in []ResourceSampleInput) error {
	if len(in) == 0 {
		return nil
	}
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		b := &pgx.Batch{}
		for _, r := range in {
			// date_bin ancla los cubos a una rejilla absoluta, así que TODAS las
			// réplicas del controlador caen en el mismo cubo sin coordinarse.
			b.Queue(`
				INSERT INTO resource_samples (tenant, project_id, bucket, cpu_milli, memory_bytes)
				VALUES ($1, $2, date_bin($3::interval, $4::timestamptz, TIMESTAMPTZ 'epoch'), $5, $6)
				ON CONFLICT (project_id, bucket) DO UPDATE SET
					cpu_milli    = GREATEST(resource_samples.cpu_milli, EXCLUDED.cpu_milli),
					memory_bytes = GREATEST(resource_samples.memory_bytes, EXCLUDED.memory_bytes)`,
				r.Tenant, r.ProjectID, SampleBucket, r.At.UTC(), r.CPUMilli, r.MemoryBytes)
		}
		br := tx.SendBatch(ctx, b)
		defer br.Close()
		for range in {
			if _, err := br.Exec(); err != nil {
				return fmtErr("registrar muestras de uso", err)
			}
		}
		return nil
	})
}

// ListResourceSamples devuelve las muestras de un proyecto desde `since`, en
// orden cronológico. Va bajo la política POR TENANT (no la de mantenimiento): lo
// consume la API con el contexto del usuario, así que la RLS es la que impide que
// un inquilino lea el consumo de otro.
func (s *Store) ListResourceSamples(ctx context.Context, projectID uuid.UUID, since time.Time) ([]domain.ResourceSample, error) {
	var out []domain.ResourceSample
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT bucket, cpu_milli, memory_bytes
			FROM resource_samples
			WHERE project_id = $1 AND bucket >= $2
			ORDER BY bucket`, projectID, since.UTC())
		if err != nil {
			return fmtErr("listar muestras de uso", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m domain.ResourceSample
			if err := rows.Scan(&m.At, &m.CPUMilli, &m.MemoryBytes); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// PurgeResourceSamples borra las muestras anteriores a `before` y devuelve
// cuántas. Sin esto la tabla crece sin techo: son 96 filas por proyecto y día.
// La ventana que el motor necesita son tres días; el resto se conserva solo para
// poder mirar la evolución, así que caduca pronto.
func (s *Store) PurgeResourceSamples(ctx context.Context, before time.Time) (int64, error) {
	var n int64
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM resource_samples WHERE bucket < $1`, before.UTC())
		if err != nil {
			return fmtErr("purgar muestras de uso", err)
		}
		n = tag.RowsAffected()
		return nil
	})
	return n, err
}
