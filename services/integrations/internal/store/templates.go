// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// GetTemplate carga UNA plantilla habilitada por su clave (para materializar una
// instancia: leer su manifiesto). ErrNotFound si no existe o está deshabilitada.
// app_templates es un catálogo GLOBAL (sin RLS); se lee bajo el rol de aplicación.
func (s *Store) GetTemplate(ctx context.Context, key string) (domain.AppTemplate, error) {
	var t domain.AppTemplate
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT key, manifest, tier, enabled FROM app_templates WHERE key = $1 AND enabled = true`,
			key).Scan(&t.Key, &t.Manifest, &t.Tier, &t.Enabled)
	})
	if isNoRows(err) {
		return domain.AppTemplate{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.AppTemplate{}, fmt.Errorf("store: obtener plantilla: %w", err)
	}
	return t, nil
}

// UpsertTemplate inserta o actualiza una plantilla del catálogo GLOBAL (semilla al
// arrancar). Idempotente por clave. No es por-org (app_templates no tiene RLS), así que
// va en una transacción SIN tenant, solo con el rol de aplicación (que tiene los GRANT).
func (s *Store) UpsertTemplate(ctx context.Context, key string, manifestJSON json.RawMessage, tier string) error {
	if tier == "" {
		tier = "standard"
	}
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		// Solo si Resolve comprobó que el rol se PUEDE asumir; si no, el
		// aislamiento lo sostiene la RLS forzada (ver platform/pkg/pg.AppRole).
		if s.rol.Activo() {
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO app_templates (key, manifest, tier, enabled, updated_at)
			VALUES ($1, $2, $3, true, now())
			ON CONFLICT (key) DO UPDATE
			   SET manifest = EXCLUDED.manifest, tier = EXCLUDED.tier,
			       updated_at = now()`,
			key, coalesceJSON(manifestJSON), tier)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: upsert plantilla %q: %w", key, err)
	}
	return nil
}

// SetTemplateEnabled habilita o deshabilita una plantilla en el catálogo global.
func (s *Store) SetTemplateEnabled(ctx context.Context, key string, enabled bool) error {
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if s.rol.Activo() {
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
				return err
			}
		}
		ct, err := tx.Exec(ctx, `UPDATE app_templates SET enabled = $1, updated_at = now() WHERE key = $2`, enabled, key)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("store: set template enabled %q: %w", key, err)
	}
	return nil
}

// ListAllTemplates devuelve todas las plantillas del catálogo global (habilitadas y
// deshabilitadas) para la superficie de operador de plataforma.
func (s *Store) ListAllTemplates(ctx context.Context) ([]domain.AppTemplate, error) {
	out := make([]domain.AppTemplate, 0, 32)
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if s.rol.Activo() {
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
				return fmt.Errorf("store: fijar rol de aplicación: %w", err)
			}
		}
		rows, err := tx.Query(ctx,
			`SELECT key, manifest, tier, enabled FROM app_templates ORDER BY key ASC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t domain.AppTemplate
			if err := rows.Scan(&t.Key, &t.Manifest, &t.Tier, &t.Enabled); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar todas las plantillas: %w", err)
	}
	return out, nil
}
