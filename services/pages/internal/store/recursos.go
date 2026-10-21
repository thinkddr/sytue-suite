// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// GuardarRecurso inserta o devuelve el recurso existente por (site_id, file).
func (s *Store) GuardarRecurso(ctx context.Context, r domain.Recurso) (domain.Recurso, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.Recurso{}, domain.ErrNoExiste
	}
	res := r
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		var sitio uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM sites WHERE id=$1 AND tenant=$2 AND deleted_at IS NULL FOR KEY SHARE`, r.SiteID, tenant.Slug).Scan(&sitio); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNoExiste
			}
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO assets (tenant, site_id, file, ext, width, height, bytes, variants, origin, name, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (site_id, file) DO UPDATE SET name = EXCLUDED.name
			RETURNING id, created_at`,
			tenant.Slug, r.SiteID, r.File, r.Ext, r.Width, r.Height, r.Bytes, r.Variants, r.Origin, r.Name, r.CreatedBy,
		)
		return row.Scan(&res.ID, &res.CreatedAt)
	})
	return res, err
}

// ListarRecursos devuelve los recursos asociados a un sitio ordenados cronológicamente.
func (s *Store) ListarRecursos(ctx context.Context, siteID uuid.UUID) ([]domain.Recurso, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return nil, domain.ErrNoExiste
	}
	var out []domain.Recurso
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, site_id, file, ext, width, height, bytes, variants, origin, name, created_by, created_at
			  FROM assets
			 WHERE site_id = $1 AND tenant = $2
			 ORDER BY created_at DESC`, siteID, tenant.Slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item domain.Recurso
			if err := rows.Scan(
				&item.ID, &item.SiteID, &item.File, &item.Ext, &item.Width, &item.Height,
				&item.Bytes, &item.Variants, &item.Origin, &item.Name, &item.CreatedBy, &item.CreatedAt,
			); err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

// RecursoPorID devuelve un recurso específico de un sitio.
func (s *Store) RecursoPorID(ctx context.Context, siteID, assetID uuid.UUID) (domain.Recurso, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.Recurso{}, domain.ErrNoExiste
	}
	var item domain.Recurso
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id, site_id, file, ext, width, height, bytes, variants, origin, name, created_by, created_at
			  FROM assets
			 WHERE id = $1 AND site_id = $2 AND tenant = $3`, assetID, siteID, tenant.Slug).
			Scan(&item.ID, &item.SiteID, &item.File, &item.Ext, &item.Width, &item.Height,
				&item.Bytes, &item.Variants, &item.Origin, &item.Name, &item.CreatedBy, &item.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoExiste
		}
		return err
	})
	return item, err
}

// BorrarRecurso borra el recurso si el borrador del sitio no lo usa. `soltar` recibe
// el recurso dentro de la misma transacción, antes de confirmarla: es quien borra sus
// ficheros del almacén, y si falla, la fila se queda y el borrado se puede repetir.
// Antes la fila se iba y los ficheros seguían sirviéndose en su URL pública (WEB-04).
//
// Si la versión publicada lo usa, borrarlo hace que se deje de ver en la web: sin
// `confirmado`, ErrRecursoPublicado, y quien lo pide avisa y pregunta (decisión del
// dueño: «borrar de verdad, avisando»).
func (s *Store) BorrarRecurso(ctx context.Context, siteID, assetID uuid.UUID, confirmado bool, soltar func(context.Context, domain.Recurso) error) error {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.ErrNoExiste
	}
	return s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		rec := domain.Recurso{ID: assetID, SiteID: siteID}
		err := tx.QueryRow(ctx, `
			SELECT file, ext, variants FROM assets
			 WHERE id = $1 AND site_id = $2 AND tenant = $3
			   FOR UPDATE`, assetID, siteID, tenant.Slug).Scan(&rec.File, &rec.Ext, &rec.Variants)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoExiste
		}
		if err != nil {
			return err
		}

		// Comprobar si el archivo está referenciado en el borrador del sitio.
		var enUso bool
		err = tx.QueryRow(ctx, `
			SELECT draft::text LIKE '%' || $1 || '%'
			  FROM sites
			 WHERE id = $2 AND tenant = $3 AND deleted_at IS NULL`, rec.File, siteID, tenant.Slug).Scan(&enUso)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if enUso {
			return domain.ErrRecursoEnUso
		}
		if !confirmado {
			var publicado bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1
					  FROM sites s
					  JOIN publications p ON p.site_id = s.id AND p.tenant = s.tenant AND p.kind = 'live' AND p.number = s.published_number
					  JOIN site_versions v ON v.id = p.version_id AND v.tenant = s.tenant
					 WHERE s.id = $2 AND s.tenant = $3 AND v.ir::text LIKE '%' || $1 || '%')`,
				rec.File, siteID, tenant.Slug).Scan(&publicado); err != nil {
				return err
			}
			if publicado {
				return domain.ErrRecursoPublicado
			}
		}

		tag, err := tx.Exec(ctx, `
			DELETE FROM assets
			 WHERE id = $1 AND site_id = $2 AND tenant = $3`, assetID, siteID, tenant.Slug)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return soltar(ctx, rec)
	})
}
