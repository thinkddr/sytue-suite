// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// DatosSitioMapa contiene los campos de un sitio para la generación del mapa.
type DatosSitioMapa struct {
	ID              uuid.UUID
	Tenant          string
	Slug            string
	Status          string
	PublishedNumber *int
	CanonicalHost   *string
	LanguagesRaw    []byte
}

// DatosPreviewMapa contiene una vista previa activa de un sitio.
type DatosPreviewMapa struct {
	SiteID     uuid.UUID
	PreviewKey string
	ExpiresAt  time.Time
}

// DatosDominioMapa contiene un host asociado a un sitio.
type DatosDominioMapa struct {
	SiteID uuid.UUID
	Host   string
	Status string
}

// DatosRetiredSlugMapa contiene un slug retirado aún vigente.
type DatosRetiredSlugMapa struct {
	SiteID uuid.UUID
	Slug   string
}

// ObtenerDatosParaMapa extrae en una transacción de mantenimiento todos los datos necesarios para generar hosts.json.
func (s *Store) ObtenerDatosParaMapa(ctx context.Context) ([]DatosSitioMapa, []DatosPreviewMapa, []DatosDominioMapa, []DatosRetiredSlugMapa, error) {
	var (
		sitios       []DatosSitioMapa
		previews     []DatosPreviewMapa
		dominios     []DatosDominioMapa
		retiredSlugs []DatosRetiredSlugMapa
	)

	err := s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		var err error
		if sitios, err = s.leerSitiosParaMapa(ctx, tx); err != nil {
			return err
		}
		if previews, err = s.leerPreviewsParaMapa(ctx, tx); err != nil {
			return err
		}
		if dominios, err = s.leerDominiosParaMapa(ctx, tx); err != nil {
			return err
		}
		if retiredSlugs, err = s.leerRetiredSlugsParaMapa(ctx, tx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return sitios, previews, dominios, retiredSlugs, nil
}

func (s *Store) leerSitiosParaMapa(ctx context.Context, tx pgx.Tx) ([]DatosSitioMapa, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, tenant, slug, status, published_number, canonical_host, draft->'languages'
		  FROM sites
		 WHERE deleted_at IS NULL
		 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []DatosSitioMapa
	for rows.Next() {
		var sm DatosSitioMapa
		if err := rows.Scan(&sm.ID, &sm.Tenant, &sm.Slug, &sm.Status, &sm.PublishedNumber, &sm.CanonicalHost, &sm.LanguagesRaw); err != nil {
			return nil, err
		}
		res = append(res, sm)
	}
	return res, rows.Err()
}

func (s *Store) leerPreviewsParaMapa(ctx context.Context, tx pgx.Tx) ([]DatosPreviewMapa, error) {
	rows, err := tx.Query(ctx, `
		SELECT site_id, preview_key, expires_at
		  FROM publications
		 WHERE kind = 'preview'
		   AND expires_at > now()
		   AND preview_key IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []DatosPreviewMapa
	for rows.Next() {
		var pm DatosPreviewMapa
		if err := rows.Scan(&pm.SiteID, &pm.PreviewKey, &pm.ExpiresAt); err != nil {
			return nil, err
		}
		res = append(res, pm)
	}
	return res, rows.Err()
}

func (s *Store) leerDominiosParaMapa(ctx context.Context, tx pgx.Tx) ([]DatosDominioMapa, error) {
	rows, err := tx.Query(ctx, `
		SELECT site_id, host, status
		  FROM domains
		 WHERE status IN ('verified', 'active')
		 ORDER BY host ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []DatosDominioMapa
	for rows.Next() {
		var dm DatosDominioMapa
		if err := rows.Scan(&dm.SiteID, &dm.Host, &dm.Status); err != nil {
			return nil, err
		}
		res = append(res, dm)
	}
	return res, rows.Err()
}

func (s *Store) leerRetiredSlugsParaMapa(ctx context.Context, tx pgx.Tx) ([]DatosRetiredSlugMapa, error) {
	rows, err := tx.Query(ctx, `
		SELECT site_id, slug
		  FROM retired_slugs
		 WHERE site_id IS NOT NULL
		   AND until > now()
		 ORDER BY slug ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []DatosRetiredSlugMapa
	for rows.Next() {
		var rm DatosRetiredSlugMapa
		if err := rows.Scan(&rm.SiteID, &rm.Slug); err != nil {
			return nil, err
		}
		res = append(res, rm)
	}
	return res, rows.Err()
}

// CrearDominio registra un dominio propio para un sitio en estado pending.
func (s *Store) CrearDominio(ctx context.Context, d *domain.Domain) error {
	return s.enSitio(ctx, d.Tenant, d.SiteID, func(tx pgx.Tx) error {
		now := time.Now().UTC()
		if d.ID == uuid.Nil {
			d.ID = uuid.New()
		}
		d.CreatedAt = now
		d.UpdatedAt = now
		if d.Status == "" {
			d.Status = domain.DomainStatusPending
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO domains (id, tenant, site_id, host, parent_id, token, status, detail, created_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			d.ID, d.Tenant, d.SiteID, d.Host, d.ParentID, d.Token, d.Status, d.Detail, d.CreatedBy, d.CreatedAt, d.UpdatedAt,
		)
		return err
	})
}

// ListarDominiosPorSitio devuelve los dominios configurados para un sitio.
func (s *Store) ListarDominiosPorSitio(ctx context.Context, tenant string, siteID uuid.UUID) ([]domain.Domain, error) {
	var list []domain.Domain
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, site_id, host, parent_id, token, status, detail,
			       checked_at, verified_at, activated_at, revalidated_at,
			       created_by, created_at, updated_at
			  FROM domains
			 WHERE site_id = $1 AND tenant = $2
			 ORDER BY created_at ASC`, siteID, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var d domain.Domain
			if err := rows.Scan(
				&d.ID, &d.Tenant, &d.SiteID, &d.Host, &d.ParentID, &d.Token, &d.Status, &d.Detail,
				&d.CheckedAt, &d.VerifiedAt, &d.ActivatedAt, &d.RevalidatedAt,
				&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, d)
		}
		return rows.Err()
	})
	return list, err
}

// CrearDenunciaAbuso inserta un reporte de abuso (sin tenant, operador) y devuelve
// cuántas lleva ese sitio en la última hora, contando esta: el aviso por correo sale
// solo con la primera de cada hora (WEB-05).
func (s *Store) CrearDenunciaAbuso(ctx context.Context, r *domain.AbuseReport) (int, error) {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	r.CreatedAt = time.Now().UTC()
	if r.Status == "" {
		r.Status = domain.AbuseReportStatusOpen
	}

	// La consulta de fuera no ve la fila que inserta la CTE (misma instantánea): +1.
	var enLaHora int
	err := s.pool.QueryRow(ctx, `
		WITH nueva AS (
			INSERT INTO abuse_reports (id, site_id, host, path, reason, details, reporter_email, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING site_id
		)
		SELECT 1 + count(*) FROM abuse_reports
		 WHERE site_id = (SELECT site_id FROM nueva) AND created_at > now() - interval '1 hour'`,
		r.ID, r.SiteID, r.Host, r.Path, r.Reason, r.Details, r.ReporterEmail, r.Status, r.CreatedAt,
	).Scan(&enLaHora)
	return enLaHora, err
}

// ObtenerDominio devuelve un dominio propio de un sitio por su ID.
func (s *Store) ObtenerDominio(ctx context.Context, tenant string, siteID, domainID uuid.UUID) (*domain.Domain, error) {
	if s.pool == nil {
		return nil, domain.ErrNoExiste
	}
	var d domain.Domain
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, tenant, site_id, host, parent_id, token, status, detail,
			       checked_at, verified_at, activated_at, revalidated_at,
			       created_by, created_at, updated_at
			  FROM domains
			 WHERE id = $1 AND site_id = $2 AND tenant = $3`, domainID, siteID, tenant).Scan(
			&d.ID, &d.Tenant, &d.SiteID, &d.Host, &d.ParentID, &d.Token, &d.Status, &d.Detail,
			&d.CheckedAt, &d.VerifiedAt, &d.ActivatedAt, &d.RevalidatedAt,
			&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
		)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNoExiste
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ActualizarDominio actualiza el estado, comprobaciones y metadatos de un dominio.
func (s *Store) ActualizarDominio(ctx context.Context, d *domain.Domain) error {
	if s.pool == nil {
		return nil
	}
	return s.enSitio(ctx, d.Tenant, d.SiteID, func(tx pgx.Tx) error {
		d.UpdatedAt = time.Now().UTC()
		tag, err := tx.Exec(ctx, `
			UPDATE domains
			   SET status = $1, detail = $2, checked_at = $3, verified_at = $4,
			       activated_at = $5, revalidated_at = $6, updated_at = $7
			 WHERE id = $8 AND site_id = $9 AND tenant = $10`,
			d.Status, d.Detail, d.CheckedAt, d.VerifiedAt, d.ActivatedAt, d.RevalidatedAt, d.UpdatedAt,
			d.ID, d.SiteID, d.Tenant,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		// Un dominio que deja de estar activo deja de ser la dirección principal.
		return soltarCanonicos(ctx, tx, d.Tenant, d.SiteID)
	})
}

// BorrarDominio elimina un dominio y su par (si es ápex con www o viceversa). Si era la
// dirección principal del sitio, deja de serlo en la misma transacción.
func (s *Store) BorrarDominio(ctx context.Context, tenant string, siteID, domainID uuid.UUID) error {
	if s.pool == nil {
		return nil
	}
	return s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM domains
			 WHERE site_id = $1 AND tenant = $3 AND (id = $2 OR parent_id = $2)`, siteID, domainID, tenant)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return soltarCanonicos(ctx, tx, tenant, siteID)
	})
}

// ContarGruposDominios cuenta los dominios raíz (parent_id IS NULL) para el límite de 5 por sitio.
func (s *Store) ContarGruposDominios(ctx context.Context, tenant string, siteID uuid.UUID) (int, error) {
	if s.pool == nil {
		return 0, nil
	}
	var count int
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT COUNT(*)
			  FROM domains
			 WHERE site_id = $1 AND tenant = $2 AND parent_id IS NULL`, siteID, tenant).Scan(&count)
	})
	return count, err
}

// DominioEnUso comprueba si un host ya está en uso verificado o activo en la plataforma.
func (s *Store) DominioEnUso(ctx context.Context, host string) (bool, error) {
	if s.pool == nil {
		return false, nil
	}
	var existe bool
	err := s.enPublico(ctx, func(tx pgx.Tx) error {
		var id uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM domains WHERE host = $1 AND status IN ('verified', 'active')`, host).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		existe = true
		return nil
	})
	return existe, err
}

// EsDominioVerificado comprueba en modo público si un host está verificado (para el ask de Caddy).
func (s *Store) EsDominioVerificado(ctx context.Context, host string) (bool, error) {
	if s.pool == nil {
		return false, nil
	}
	var existe bool
	err := s.enPublico(ctx, func(tx pgx.Tx) error {
		var id uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM domains WHERE host = $1 AND status = 'verified'`, host).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		existe = true
		return nil
	})
	return existe, err
}

// ResolverHostPublico resuelve en modo público un Host a su tenant y site_id: el del
// dominio ACTIVO con ese host o, si es `<slug>.<sitesDomain>`, el del sitio con ese
// slug. `sites.canonical_host` no resuelve nada por sí mismo: quien lo escribía se
// quedaba con el host (WEB-01), y un canónico legítimo es siempre un dominio activo
// del sitio, que ya sale por aquí. Sin la rama del slug, ninguna web servida en su
// dirección <slug>.sites.<dominio> podía usar /_pages (tienda, formularios, denuncias).
func (s *Store) ResolverHostPublico(ctx context.Context, host, sitesDomain string) (string, uuid.UUID, error) {
	if s.pool == nil {
		return "", uuid.Nil, domain.ErrNoExiste
	}
	var (
		tenant string
		siteID uuid.UUID
	)
	slug, esSlug := strings.CutSuffix(host, "."+sitesDomain)
	esSlug = esSlug && sitesDomain != "" && slug != "" && !strings.Contains(slug, ".")
	err := s.enPublico(ctx, func(tx pgx.Tx) error {
		if esSlug {
			return tx.QueryRow(ctx, `
				SELECT tenant, id
				  FROM sites
				 WHERE slug = $1 AND deleted_at IS NULL AND status = 'active'`, slug).Scan(&tenant, &siteID)
		}
		return tx.QueryRow(ctx, `
			SELECT d.tenant, d.site_id
			  FROM domains d
			  JOIN sites s ON s.id = d.site_id AND s.tenant = d.tenant
			 WHERE d.host = $1 AND d.status = 'active' AND s.deleted_at IS NULL AND s.status = 'active'`, host).Scan(&tenant, &siteID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", uuid.Nil, domain.ErrNoExiste
	}
	return tenant, siteID, err
}

// RegistrarAltcha registra una firma ALTCHA usada. Devuelve true si era nueva.
func (s *Store) RegistrarAltcha(ctx context.Context, firma string, caduca time.Time) (bool, error) {
	if s.pool == nil {
		return true, nil
	}
	// La tabla guarda el HASH de la firma (signature_hash, migración 0001); antes el
	// INSERT nombraba una columna `signature` que no existe y TODA comprobación de
	// ALTCHA fallaba (checkout, formularios y desistimientos: altcha_invalid).
	suma := sha256.Sum256([]byte(firma))
	_, _ = s.pool.Exec(ctx, `DELETE FROM altcha_used WHERE expires_at < now() - interval '1 hour'`)
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO altcha_used (signature_hash, expires_at)
		VALUES ($1, $2)
		ON CONFLICT (signature_hash) DO NOTHING`,
		hex.EncodeToString(suma[:]), caduca,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ObtenerDominiosPendientesBarrido devuelve los dominios pendientes para el barrido periódico.
func (s *Store) ObtenerDominiosPendientesBarrido(ctx context.Context) ([]domain.Domain, error) {
	if s.pool == nil {
		return nil, nil
	}
	var res []domain.Domain
	err := s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, site_id, host, parent_id, token, status, detail,
			       checked_at, verified_at, activated_at, revalidated_at,
			       created_by, created_at, updated_at
			  FROM domains
			 WHERE status = 'pending'
			 ORDER BY created_at ASC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d domain.Domain
			if err := rows.Scan(
				&d.ID, &d.Tenant, &d.SiteID, &d.Host, &d.ParentID, &d.Token, &d.Status, &d.Detail,
				&d.CheckedAt, &d.VerifiedAt, &d.ActivatedAt, &d.RevalidatedAt,
				&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
			); err != nil {
				return err
			}
			res = append(res, d)
		}
		return rows.Err()
	})
	return res, err
}

// ObtenerDominiosActivosRevalidacion devuelve los dominios activos para la revalidación diaria.
func (s *Store) ObtenerDominiosActivosRevalidacion(ctx context.Context) ([]domain.Domain, error) {
	if s.pool == nil {
		return nil, nil
	}
	var res []domain.Domain
	err := s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, site_id, host, parent_id, token, status, detail,
			       checked_at, verified_at, activated_at, revalidated_at,
			       created_by, created_at, updated_at
			  FROM domains
			 WHERE status = 'active'
			 ORDER BY created_at ASC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d domain.Domain
			if err := rows.Scan(
				&d.ID, &d.Tenant, &d.SiteID, &d.Host, &d.ParentID, &d.Token, &d.Status, &d.Detail,
				&d.CheckedAt, &d.VerifiedAt, &d.ActivatedAt, &d.RevalidatedAt,
				&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
			); err != nil {
				return err
			}
			res = append(res, d)
		}
		return rows.Err()
	})
	return res, err
}

// EliminarDominioPorID elimina un dominio definitivamente por su ID.
func (s *Store) EliminarDominioPorID(ctx context.Context, tenant string, id uuid.UUID) error {
	if s.pool == nil {
		return nil
	}
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM domains WHERE id = $1 AND tenant = $2`, id, tenant)
		return err
	})
}
