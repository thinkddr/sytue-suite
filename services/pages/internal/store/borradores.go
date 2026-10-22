// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// Borrador devuelve el IR exacto que se guardó, dentro del tenant activo.
func (s *Store) Borrador(ctx context.Context, id uuid.UUID) (domain.Sitio, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.Sitio{}, domain.ErrNoExiste
	}
	var sitio domain.Sitio
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id, slug, name, template, draft, draft_rev FROM sites WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`, id, tenant.Slug).
			Scan(&sitio.ID, &sitio.Slug, &sitio.Name, &sitio.Template, &sitio.Draft, &sitio.DraftRev)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Sitio{}, domain.ErrNoExiste
	}
	return sitio, err
}

// GuardarBorrador hace el compare-and-swap de draft_rev. El servicio valida el
// JSON antes de llamar: Store únicamente preserva bytes JSON válidos.
func (s *Store) GuardarBorrador(ctx context.Context, id uuid.UUID, rev int64, ir json.RawMessage, sub string) (int64, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || rev < 1 || !json.Valid(ir) || sub == "" {
		return 0, domain.ErrBorradorInvalido
	}
	var nueva int64
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE sites SET draft = $1, draft_rev = draft_rev + 1, draft_updated_by = $2, draft_updated_at = now(), updated_at = now()
			WHERE id = $3 AND tenant = $4 AND draft_rev = $5 AND deleted_at IS NULL RETURNING draft_rev`, ir, sub, id, tenant.Slug, rev).Scan(&nueva)
		if errors.Is(err, pgx.ErrNoRows) {
			var existe bool
			if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sites WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL)`, id, tenant.Slug).Scan(&existe); e != nil {
				return e
			}
			if !existe {
				return domain.ErrNoExiste
			}
			return domain.ErrConflictoBorrador
		}
		return err
	})
	return nueva, err
}

// CrearVersion congela el borrador actual bajo un nombre opcional.
func (s *Store) CrearVersion(ctx context.Context, id uuid.UUID, nombre, sub string) (domain.VersionSitio, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || sub == "" {
		return domain.VersionSitio{}, domain.ErrNoExiste
	}
	nombre, err = domain.NormalizarNombreVersion(nombre)
	if err != nil {
		return domain.VersionSitio{}, err
	}
	var version domain.VersionSitio
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO site_versions (tenant, site_id, name, kind, ir, draft_rev, created_by)
			SELECT tenant, id, $1, 'manual', draft, draft_rev, $2 FROM sites WHERE id = $3 AND tenant = $4 AND deleted_at IS NULL
			RETURNING id, name, kind, draft_rev`, nombre, sub, id, tenant.Slug).Scan(&version.ID, &version.Name, &version.Kind, &version.DraftRev)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.VersionSitio{}, domain.ErrNoExiste
	}
	return version, err
}
