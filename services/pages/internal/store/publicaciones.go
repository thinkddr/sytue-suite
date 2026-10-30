// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// SitioCompleto devuelve toda la información de un sitio dentro del tenant activo.
func (s *Store) SitioCompleto(ctx context.Context, id uuid.UUID) (domain.SitioCompleto, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.SitioCompleto{}, domain.ErrNoExiste
	}

	var sc domain.SitioCompleto
	var rawLegal []byte
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant, slug, name, template, status, suspended_reason, draft, draft_rev,
			       published_number, canonical_host, legal, analytics_enabled, analytics_site_id,
			       export_origins, export_app_schemes, created_at, updated_at, deleted_at
			FROM sites
			WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`,
			id, tenant.Slug,
		)
		return row.Scan(
			&sc.ID, &sc.Tenant, &sc.Slug, &sc.Name, &sc.Template, &sc.Status, &sc.SuspendedReason, &sc.Draft, &sc.DraftRev,
			&sc.PublishedNumber, &sc.CanonicalHost, &rawLegal, &sc.AnalyticsEnabled, &sc.AnalyticsSiteID,
			&sc.ExportOrigins, &sc.ExportAppSchemes, &sc.CreatedAt, &sc.UpdatedAt, &sc.DeletedAt,
		)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SitioCompleto{}, domain.ErrNoExiste
	}
	if err != nil {
		return domain.SitioCompleto{}, err
	}
	if len(rawLegal) > 0 {
		_ = json.Unmarshal(rawLegal, &sc.Legal)
	}
	return sc, nil
}

// CrearVersionConTipo crea una entrada en site_versions con un tipo específico (publish, preview, export, manual).
func (s *Store) CrearVersionConTipo(ctx context.Context, id uuid.UUID, nombre, kind string, irData json.RawMessage, rev int64, sub string) (uuid.UUID, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || sub == "" {
		return uuid.Nil, domain.ErrNoExiste
	}
	nombre, err = domain.NormalizarNombreVersion(nombre)
	if err != nil {
		return uuid.Nil, err
	}

	var versionID uuid.UUID
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO site_versions (tenant, site_id, name, kind, ir, draft_rev, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`,
			tenant.Slug, id, nombre, kind, irData, rev, sub,
		).Scan(&versionID)
	})
	if err != nil {
		return uuid.Nil, err
	}
	return versionID, nil
}

// Version obtiene una versión específica del sitio.
func (s *Store) Version(ctx context.Context, siteID, vid uuid.UUID) (domain.VersionSitio, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.VersionSitio{}, domain.ErrNoExiste
	}

	var v domain.VersionSitio
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, name, kind, ir, draft_rev, created_by, created_at
			FROM site_versions
			WHERE id = $1 AND site_id = $2 AND tenant = $3`,
			vid, siteID, tenant.Slug,
		).Scan(&v.ID, &v.Name, &v.Kind, &v.IR, &v.DraftRev, &v.CreatedBy, &v.CreatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.VersionSitio{}, domain.ErrNoExiste
	}
	return v, err
}

// ContarVistasVivas devuelve el número de vistas previas activas (no caducadas) para un sitio.
func (s *Store) ContarVistasVivas(ctx context.Context, siteID uuid.UUID) (int, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return 0, domain.ErrNoExiste
	}

	var count int
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM publications
			WHERE site_id = $1 AND tenant = $2 AND kind = 'preview' AND expires_at > now()`,
			siteID, tenant.Slug,
		).Scan(&count)
	})
	return count, err
}

// CrearPublicacionLive realiza la publicación en vivo en una única transacción:
// calcula number, inserta la publicación, actualiza sites.published_number y registra redirecciones.
func (s *Store) CrearPublicacionLive(
	ctx context.Context,
	siteID, versionID uuid.UUID,
	files int,
	bytesTotal int64,
	rendererVersion string,
	dataHash json.RawMessage,
	sub string,
	redirects []domain.Redirect,
) (int, error) {
	return s.CrearPublicacionLiveConArchivos(ctx, siteID, versionID, files, bytesTotal, rendererVersion, dataHash, sub, redirects, nil)
}

// CrearPublicacionLiveConArchivos prepara los objetos antes de confirmar el puntero público.
func (s *Store) CrearPublicacionLiveConArchivos(
	ctx context.Context,
	siteID, versionID uuid.UUID,
	files int,
	bytesTotal int64,
	rendererVersion string,
	dataHash json.RawMessage,
	sub string,
	redirects []domain.Redirect,
	preparar func(int) error,
) (int, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return 0, domain.ErrNoExiste
	}

	if len(dataHash) == 0 {
		dataHash = json.RawMessage("{}")
	}

	var number int
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		// Serializar publicaciones del mismo sitio y conservar RLS.
		var bloqueado uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM sites WHERE id=$1 AND tenant=$2 AND deleted_at IS NULL FOR UPDATE`, siteID, tenant.Slug).Scan(&bloqueado); err != nil {
			return err
		}
		rawIR, err := asegurarVersionSitio(ctx, tx, siteID, versionID, tenant.Slug)
		if err != nil {
			return err
		}
		// Calcular siguiente número para este sitio
		err = tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(number), 0) + 1
			FROM publications
			WHERE site_id = $1 AND tenant = $2 AND kind = 'live'`,
			siteID, tenant.Slug,
		).Scan(&number)
		if err != nil {
			return fmt.Errorf("calcular número de publicación: %w", err)
		}

		if preparar != nil {
			if err := preparar(number); err != nil {
				return err
			}
		}

		// Insertar publicación
		_, err = tx.Exec(ctx, `
			INSERT INTO publications (tenant, site_id, kind, number, version_id, files, bytes, renderer_version, data_hash, created_by)
			VALUES ($1, $2, 'live', $3, $4, $5, $6, $7, $8, $9)`,
			tenant.Slug, siteID, number, versionID, files, bytesTotal, rendererVersion, dataHash, sub,
		)
		if err != nil {
			return fmt.Errorf("insertar publicación: %w", err)
		}

		// Actualizar sitio con el nuevo published_number
		tag, err := tx.Exec(ctx, `
			UPDATE sites
			SET published_number = $1, updated_at = now()
			WHERE id = $2 AND tenant = $3 AND deleted_at IS NULL`,
			number, siteID, tenant.Slug,
		)
		if err != nil {
			return fmt.Errorf("actualizar published_number: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}

		// Guardar redirecciones nuevas
		for _, red := range redirects {
			if red.FromPath != "" && red.ToPath != "" {
				_, err = tx.Exec(ctx, `
					INSERT INTO redirects (tenant, site_id, from_path, to_path)
					VALUES ($1, $2, $3, $4)
					ON CONFLICT (site_id, from_path) DO UPDATE SET to_path = EXCLUDED.to_path`,
					tenant.Slug, siteID, red.FromPath, red.ToPath,
				)
				if err != nil {
					return fmt.Errorf("insertar redirección (%s -> %s): %w", red.FromPath, red.ToPath, err)
				}
			}
		}

		return registrarFormulariosPublicados(ctx, tx, tenant.Slug, siteID, rawIR)
	})

	return number, err
}

// CrearPublicacionPreview inserta una publicación de tipo preview con clave y caducidad.
func (s *Store) CrearPublicacionPreview(
	ctx context.Context,
	siteID, versionID uuid.UUID,
	previewKey string,
	expiresAt time.Time,
	files int,
	bytesTotal int64,
	rendererVersion string,
	dataHash json.RawMessage,
	sub string,
) error {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.ErrNoExiste
	}

	if len(dataHash) == 0 {
		dataHash = json.RawMessage("{}")
	}

	return s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		if _, err := asegurarVersionSitio(ctx, tx, siteID, versionID, tenant.Slug); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO publications (tenant, site_id, kind, preview_key, expires_at, version_id, files, bytes, renderer_version, data_hash, created_by)
			VALUES ($1, $2, 'preview', $3, $4, $5, $6, $7, $8, $9, $10)`,
			tenant.Slug, siteID, previewKey, expiresAt, versionID, files, bytesTotal, rendererVersion, dataHash, sub,
		)
		return err
	})
}

// ListarPublicaciones devuelve el historial de publicaciones para el sitio.
func (s *Store) ListarPublicaciones(ctx context.Context, siteID uuid.UUID) ([]domain.Publicacion, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return nil, domain.ErrNoExiste
	}

	sitio, err := s.SitioCompleto(ctx, siteID)
	if err != nil {
		return nil, err
	}

	var pubs []domain.Publicacion
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, site_id, kind, number, preview_key, expires_at, version_id, files, bytes, renderer_version, data_hash, created_by, created_at
			FROM publications
			WHERE site_id = $1 AND tenant = $2
			ORDER BY created_at DESC`,
			siteID, tenant.Slug,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p domain.Publicacion
			if err := rows.Scan(
				&p.ID, &p.SiteID, &p.Kind, &p.Number, &p.PreviewKey, &p.ExpiresAt,
				&p.VersionID, &p.Files, &p.Bytes, &p.RendererVersion, &p.DataHash, &p.CreatedBy, &p.CreatedAt,
			); err != nil {
				return err
			}
			if p.Number != nil && sitio.PublishedNumber != nil && *p.Number == *sitio.PublishedNumber {
				p.Current = true
			}
			pubs = append(pubs, p)
		}
		return rows.Err()
	})
	if pubs == nil {
		pubs = []domain.Publicacion{}
	}
	return pubs, err
}

// Rollback mueve el published_number de un sitio a una publicación live previa existente.
func (s *Store) Rollback(ctx context.Context, siteID uuid.UUID, number int) error {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.ErrNoExiste
	}

	return s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		var versionID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT version_id FROM publications WHERE site_id = $1 AND tenant = $2 AND kind = 'live' AND number = $3`,
			siteID, tenant.Slug, number,
		).Scan(&versionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoExiste
		}
		if err != nil {
			return err
		}
		rawIR, err := asegurarVersionSitio(ctx, tx, siteID, versionID, tenant.Slug)
		if err != nil {
			return err
		}

		tag, err := tx.Exec(ctx, `
			UPDATE sites
			SET published_number = $1, updated_at = now()
			WHERE id = $2 AND tenant = $3 AND deleted_at IS NULL`,
			number, siteID, tenant.Slug,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return registrarFormulariosPublicados(ctx, tx, tenant.Slug, siteID, rawIR)
	})
}

// Despublicar retira la publicación vigente y revoca sus previews en la misma transacción.
func (s *Store) Despublicar(ctx context.Context, siteID uuid.UUID) error {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.ErrNoExiste
	}

	return s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE sites
			SET published_number = NULL, updated_at = now()
			WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`,
			siteID, tenant.Slug,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		_, err = tx.Exec(ctx, `
			UPDATE publications SET expires_at = now()
			WHERE site_id = $1 AND tenant = $2 AND kind = 'preview' AND expires_at > now()`,
			siteID, tenant.Slug,
		)
		return err
	})
}

// PublicacionVigente devuelve la publicación actualmente en vivo para el sitio.
func (s *Store) PublicacionVigente(ctx context.Context, siteID uuid.UUID) (domain.Publicacion, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.Publicacion{}, domain.ErrNoExiste
	}

	sitio, err := s.SitioCompleto(ctx, siteID)
	if err != nil {
		return domain.Publicacion{}, err
	}
	if sitio.PublishedNumber == nil {
		return domain.Publicacion{}, domain.ErrNoExiste
	}

	var p domain.Publicacion
	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, site_id, kind, number, preview_key, expires_at, version_id, files, bytes, renderer_version, data_hash, created_by, created_at
			FROM publications
			WHERE site_id = $1 AND tenant = $2 AND kind = 'live' AND number = $3`,
			siteID, tenant.Slug, *sitio.PublishedNumber,
		).Scan(
			&p.ID, &p.SiteID, &p.Kind, &p.Number, &p.PreviewKey, &p.ExpiresAt,
			&p.VersionID, &p.Files, &p.Bytes, &p.RendererVersion, &p.DataHash, &p.CreatedBy, &p.CreatedAt,
		)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Publicacion{}, domain.ErrNoExiste
	}
	if err != nil {
		return domain.Publicacion{}, err
	}
	p.Current = true
	return p, nil
}

// ExportarDatosHub obtiene los sitios y versiones creados por el sujeto para el Hub (PAGES-ENCARGO §6.3, GET /export).
func (s *Store) ExportarDatosHub(ctx context.Context, sub string) ([]domain.SitioCompleto, []domain.VersionSitio, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		//nolint:nilerr // cuenta personal sin tenant entrega vacio intencionadamente
		return []domain.SitioCompleto{}, []domain.VersionSitio{}, nil
	}

	var sitios []domain.SitioCompleto
	var versiones []domain.VersionSitio

	err = s.enTenant(ctx, tenant.Slug, func(tx pgx.Tx) error {
		// Sitios creados por la persona dentro de su tenant
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, slug, name, template, status, suspended_reason, draft, draft_rev,
			       published_number, canonical_host, legal, analytics_enabled, analytics_site_id,
			       export_origins, export_app_schemes, created_at, updated_at, deleted_at
			FROM sites
			WHERE tenant = $1 AND created_by = $2 AND deleted_at IS NULL`,
			tenant.Slug, sub,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var sc domain.SitioCompleto
			var rawLegal []byte
			if err := rows.Scan(
				&sc.ID, &sc.Tenant, &sc.Slug, &sc.Name, &sc.Template, &sc.Status, &sc.SuspendedReason, &sc.Draft, &sc.DraftRev,
				&sc.PublishedNumber, &sc.CanonicalHost, &rawLegal, &sc.AnalyticsEnabled, &sc.AnalyticsSiteID,
				&sc.ExportOrigins, &sc.ExportAppSchemes, &sc.CreatedAt, &sc.UpdatedAt, &sc.DeletedAt,
			); err != nil {
				return err
			}
			if len(rawLegal) > 0 {
				_ = json.Unmarshal(rawLegal, &sc.Legal)
			}
			sitios = append(sitios, sc)
		}

		// Versiones creadas por la persona dentro de su tenant
		rV, err := tx.Query(ctx, `
			SELECT id, name, kind, ir, draft_rev, created_by, created_at
			FROM site_versions
			WHERE tenant = $1 AND created_by = $2`,
			tenant.Slug, sub,
		)
		if err != nil {
			return err
		}
		defer rV.Close()

		for rV.Next() {
			var v domain.VersionSitio
			if err := rV.Scan(&v.ID, &v.Name, &v.Kind, &v.IR, &v.DraftRev, &v.CreatedBy, &v.CreatedAt); err != nil {
				return err
			}
			versiones = append(versiones, v)
		}

		return nil
	})

	return sitios, versiones, err
}

// La FK por UUID no asegura por sí sola que una versión pertenezca al sitio y tenant.
func asegurarVersionSitio(ctx context.Context, tx pgx.Tx, siteID, versionID uuid.UUID, tenant string) ([]byte, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT ir FROM site_versions WHERE id=$1 AND site_id=$2 AND tenant=$3 FOR KEY SHARE`, versionID, siteID, tenant).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNoExiste
	}
	return raw, err
}

// registrarFormulariosPublicados crea las filas por defecto en form_settings para los
// formularios presentes en la versión publicada (WEB-07), conservando intactos los ajustes
// existentes (incluidos los desactivados con enabled = false o con destinatarios en notify).
func registrarFormulariosPublicados(ctx context.Context, tx pgx.Tx, tenant string, siteID uuid.UUID, rawIR []byte) error {
	if len(rawIR) == 0 {
		return nil
	}
	sitio, decErr := ir.Decodificar(rawIR)
	if decErr != nil {
		//nolint:nilerr // un IR mínimo de pruebas ("{}") no lleva páginas ni formularios
		return nil
	}
	for formID := range ir.Formularios(sitio) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO form_settings (site_id, tenant, form_id, enabled, notify)
			VALUES ($1, $2, $3, true, '[]'::jsonb)
			ON CONFLICT (site_id, form_id) DO NOTHING`,
			siteID, tenant, formID,
		); err != nil {
			return fmt.Errorf("registrar formulario publicado %s: %w", formID, err)
		}
	}
	return nil
}
