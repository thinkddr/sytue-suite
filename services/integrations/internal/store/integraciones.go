// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// DisabledIntegrations devuelve los proveedores que la organización activa ha
// apagado. Lo que no aparece está encendido.
func (s *Store) DisabledIntegrations(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT provider FROM disabled_integrations WHERE org_id = current_setting('app.tenant', true)`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return err
			}
			out[p] = true
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: integraciones apagadas: %w", err)
	}
	return out, nil
}

// SetIntegrationEnabled enciende o apaga un proveedor en la organización activa.
func (s *Store) SetIntegrationEnabled(ctx context.Context, provider string, enabled bool) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		if enabled {
			_, err := tx.Exec(ctx, `DELETE FROM disabled_integrations WHERE org_id = $1 AND provider = $2`, t.Slug, provider)
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO disabled_integrations (org_id, provider) VALUES ($1, $2) ON CONFLICT DO NOTHING`, t.Slug, provider)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: cambiar integración %s: %w", provider, err)
	}
	return nil
}
