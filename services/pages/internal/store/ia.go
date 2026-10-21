// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// GuardarClaveAI almacena la clave BYOK sellada borrando las anteriores de la organización (§9.2).
func (s *Store) GuardarClaveAI(ctx context.Context, tenant string, k domain.AIKey, keyEnc string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM ai_keys WHERE tenant = $1`, tenant); err != nil {
			return fmt.Errorf("store.GuardarClaveAI: limpiar anteriores: %w", err)
		}

		query := `
			INSERT INTO ai_keys (tenant, provider, model, key_enc, notice_accepted_at, accepted_by, checked_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now())
			RETURNING id, created_at`
		return tx.QueryRow(ctx, query,
			tenant,
			k.Provider,
			k.Model,
			keyEnc,
			k.NoticeAcceptedAt,
			k.AcceptedBy,
			k.CheckedAt,
		).Scan(&k.ID, &k.CreatedAt)
	})
}

// ObtenerClaveAIVigente obtiene la clave vigente más reciente del tenant junto con su texto cifrado.
func (s *Store) ObtenerClaveAIVigente(ctx context.Context, tenant string) (*domain.AIKey, string, error) {
	var k domain.AIKey
	var keyEnc string
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		query := `
			SELECT id, tenant, provider, model, key_enc, notice_accepted_at, accepted_by, checked_at, created_at
			FROM ai_keys
			WHERE tenant = $1
			ORDER BY created_at DESC
			LIMIT 1`
		row := tx.QueryRow(ctx, query, tenant)
		return row.Scan(
			&k.ID,
			&k.Tenant,
			&k.Provider,
			&k.Model,
			&keyEnc,
			&k.NoticeAcceptedAt,
			&k.AcceptedBy,
			&k.CheckedAt,
			&k.CreatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("store.ObtenerClaveAIVigente: %w", err)
	}
	return &k, keyEnc, nil
}

// BorrarClaveAI elimina las claves BYOK del tenant.
func (s *Store) BorrarClaveAI(ctx context.Context, tenant string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM ai_keys WHERE tenant = $1`, tenant)
		if err != nil {
			return fmt.Errorf("store.BorrarClaveAI: %w", err)
		}
		return nil
	})
}

// ContarTrabajosMes cuenta los trabajos estándar (no BYOK) iniciados desde 'desde' en el tenant.
func (s *Store) ContarTrabajosMes(ctx context.Context, tenant string, desde time.Time) (int, error) {
	var count int
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		query := `
			SELECT count(*)
			FROM ai_jobs
			WHERE tenant = $1 AND NOT byok AND created_at >= $2`
		return tx.QueryRow(ctx, query, tenant, desde).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("store.ContarTrabajosMes: %w", err)
	}
	return count, nil
}

// ContarTrabajosEnMarchaOrg cuenta cuántos trabajos running tiene actualmente la organización.
func (s *Store) ContarTrabajosEnMarchaOrg(ctx context.Context, tenant string) (int, error) {
	var count int
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		query := `
			SELECT count(*)
			FROM ai_jobs
			WHERE tenant = $1 AND status = 'running'`
		return tx.QueryRow(ctx, query, tenant).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("store.ContarTrabajosEnMarchaOrg: %w", err)
	}
	return count, nil
}

// CrearTrabajoAI registra un nuevo trabajo de IA en estado running (§9.3).
func (s *Store) CrearTrabajoAI(ctx context.Context, j *domain.AIJob) (*domain.AIJob, error) {
	err := s.enSitio(ctx, j.Tenant, j.SiteID, func(tx pgx.Tx) error {
		query := `
			INSERT INTO ai_jobs (
				tenant, site_id, kind, status, input, result_key, error,
				provider, byok, model, tokens_in, tokens_out, created_by, created_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now())
			RETURNING id, created_at`
		return tx.QueryRow(ctx, query,
			j.Tenant,
			j.SiteID,
			j.Kind,
			domain.AIJobStatusRunning,
			j.Input,
			j.ResultKey,
			j.Error,
			j.Provider,
			j.BYOK,
			j.Model,
			j.TokensIn,
			j.TokensOut,
			j.CreatedBy,
		).Scan(&j.ID, &j.CreatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("store.CrearTrabajoAI: %w", err)
	}
	j.Status = domain.AIJobStatusRunning
	return j, nil
}

// ActualizarTrabajoAI actualiza el estado, resultado, métricas y finalización de un trabajo de IA.
func (s *Store) ActualizarTrabajoAI(ctx context.Context, j *domain.AIJob) error {
	return s.enTenant(ctx, j.Tenant, func(tx pgx.Tx) error {
		query := `
			UPDATE ai_jobs
			SET status = $1, result = $2, result_key = $3, error = $4,
			    tokens_in = $5, tokens_out = $6, finished_at = $7
			WHERE id = $8 AND tenant = $9`
		cmd, err := tx.Exec(ctx, query,
			j.Status,
			j.Result,
			j.ResultKey,
			j.Error,
			j.TokensIn,
			j.TokensOut,
			j.FinishedAt,
			j.ID,
			j.Tenant,
		)
		if err != nil {
			return fmt.Errorf("store.ActualizarTrabajoAI: %w", err)
		}
		if cmd.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return nil
	})
}

// ObtenerTrabajoAI recupera un trabajo por su id y tenant.
func (s *Store) ObtenerTrabajoAI(ctx context.Context, tenant string, id uuid.UUID) (*domain.AIJob, error) {
	var j domain.AIJob
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		query := `
			SELECT id, tenant, site_id, kind, status, input, result, result_key, error,
			       provider, byok, model, tokens_in, tokens_out, created_by, created_at, finished_at
			FROM ai_jobs
			WHERE id = $1 AND tenant = $2`
		row := tx.QueryRow(ctx, query, id, tenant)
		return row.Scan(
			&j.ID,
			&j.Tenant,
			&j.SiteID,
			&j.Kind,
			&j.Status,
			&j.Input,
			&j.Result,
			&j.ResultKey,
			&j.Error,
			&j.Provider,
			&j.BYOK,
			&j.Model,
			&j.TokensIn,
			&j.TokensOut,
			&j.CreatedBy,
			&j.CreatedAt,
			&j.FinishedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNoExiste
		}
		return nil, fmt.Errorf("store.ObtenerTrabajoAI: %w", err)
	}
	return &j, nil
}

// BarrerTrabajosVencidos marca como failed ('interrumpido') aquellos trabajos running que
// superaron su plazo más 5 minutos (§9.3). Corre bajo enMantenimiento.
func (s *Store) BarrerTrabajosVencidos(ctx context.Context) (int64, error) {
	var afectados int64
	err := s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		query := `
			UPDATE ai_jobs
			SET status = 'failed', error = 'interrumpido', finished_at = now()
			WHERE status = 'running'
			  AND (
			    (kind IN ('design', 'translate') AND created_at < now() - INTERVAL '15 minutes')
			    OR
			    (kind = 'export' AND created_at < now() - INTERVAL '35 minutes')
			  )`
		cmd, err := tx.Exec(ctx, query)
		if err != nil {
			return err
		}
		afectados = cmd.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("store.BarrerTrabajosVencidos: %w", err)
	}
	return afectados, nil
}
