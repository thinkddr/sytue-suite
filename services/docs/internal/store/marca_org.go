// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

const colsMarca = `version, name, bg, ink, accent, heading_font, body_font, created_by, created_at`

func scanMarca(row pgx.Row, m *domain.MarcaOrg) error {
	return row.Scan(&m.Version, &m.Name, &m.Bg, &m.Ink, &m.Accent, &m.HeadingFont, &m.BodyFont, &m.CreatedBy, &m.CreatedAt)
}

// GuardarMarcaOrg añade una versión NUEVA de la marca de la organización (la siguiente a
// la más alta) y la devuelve; las anteriores se conservan. `m` ya viene normalizada.
func (s *Store) GuardarMarcaOrg(ctx context.Context, m domain.MarcaOrg, sub string) (*domain.MarcaOrg, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	var out domain.MarcaOrg
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		// Dos administradores guardando a la vez no pueden repetir número: se serializa por organización.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('doc_org_brand:' || $1))`, t.Slug); err != nil {
			return err
		}
		return scanMarca(tx.QueryRow(ctx, `
			INSERT INTO doc_org_brand (tenant, version, name, bg, ink, accent, heading_font, body_font, created_by)
			VALUES ($1, (SELECT coalesce(max(version), 0) + 1 FROM doc_org_brand WHERE tenant = $1), $2, $3, $4, $5, $6, $7, $8)
			RETURNING `+colsMarca, t.Slug, m.Name, m.Bg, m.Ink, m.Accent, m.HeadingFont, m.BodyFont, sub), &out)
	})
	if err != nil {
		return nil, fmt.Errorf("store: guardar marca: %w", err)
	}
	return &out, nil
}

// MarcaOrg devuelve la versión `version` de la marca de la organización; con 0, la
// vigente (la más alta). ErrNotFound si la organización no tiene marca (o esa versión).
func (s *Store) MarcaOrg(ctx context.Context, version int) (*domain.MarcaOrg, error) {
	var m domain.MarcaOrg
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return scanMarca(tx.QueryRow(ctx, `
			SELECT `+colsMarca+` FROM doc_org_brand
			WHERE $1 = 0 OR version = $1 ORDER BY version DESC LIMIT 1`, version), &m)
	})
	if isNoRows(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: obtener marca: %w", err)
	}
	return &m, nil
}

// VersionesMarcaOrg lista las versiones de la marca, la más reciente primero.
func (s *Store) VersionesMarcaOrg(ctx context.Context, limite int) ([]domain.MarcaOrg, error) {
	limite = min(max(limite, 1), 200)
	out := []domain.MarcaOrg{}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+colsMarca+` FROM doc_org_brand ORDER BY version DESC LIMIT $1`, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m domain.MarcaOrg
			if err := scanMarca(rows, &m); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: versiones de la marca: %w", err)
	}
	return out, nil
}
