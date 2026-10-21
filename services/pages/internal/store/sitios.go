// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/plantillas"
)

var reSlugValido = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ListarSitios devuelve todos los sitios de una organización. Si incluirBorrados es true,
// incluye los sitios en papelera (eliminados hace menos de 30 días).
func (s *Store) ListarSitios(ctx context.Context, tenant string, incluirBorrados bool) ([]domain.SitioCompleto, error) {
	if tenant == "" {
		return nil, domain.ErrNoExiste
	}

	var sitios []domain.SitioCompleto
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		query := `
			SELECT id, tenant, slug, name, template, status, suspended_reason, draft, draft_rev,
			       published_number, canonical_host, legal, analytics_enabled, analytics_site_id,
			       export_origins, export_app_schemes, created_at, updated_at, deleted_at
			  FROM sites
			 WHERE tenant = $1`

		if !incluirBorrados {
			query += ` AND deleted_at IS NULL`
		} else {
			query += ` AND (deleted_at IS NULL OR deleted_at >= now() - interval '30 days')`
		}
		query += ` ORDER BY created_at DESC`

		rows, err := tx.Query(ctx, query, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var sc domain.SitioCompleto
			var rawLegal []byte
			if err := rows.Scan(
				&sc.ID, &sc.Tenant, &sc.Slug, &sc.Name, &sc.Template, &sc.Status, &sc.SuspendedReason,
				&sc.Draft, &sc.DraftRev, &sc.PublishedNumber, &sc.CanonicalHost, &rawLegal,
				&sc.AnalyticsEnabled, &sc.AnalyticsSiteID, &sc.ExportOrigins, &sc.ExportAppSchemes,
				&sc.CreatedAt, &sc.UpdatedAt, &sc.DeletedAt,
			); err != nil {
				return err
			}
			if len(rawLegal) > 0 {
				_ = json.Unmarshal(rawLegal, &sc.Legal)
			}
			sitios = append(sitios, sc)
		}
		return rows.Err()
	})

	return sitios, err
}

// CrearSitio crea un nuevo sitio a partir de una plantilla o en blanco.
func (s *Store) CrearSitio(ctx context.Context, tenant string, input domain.CrearSitioInput, sub string) (*domain.SitioCompleto, error) {
	if tenant == "" || sub == "" {
		return nil, domain.ErrNoExiste
	}

	nombre := strings.TrimSpace(input.Name)
	if len(nombre) < 1 || len(nombre) > 120 {
		return nil, fmt.Errorf("%w: nombre inválido (1-120 caracteres)", domain.ErrBorradorInvalido)
	}

	slug := strings.ToLower(strings.TrimSpace(input.Slug))
	if !reSlugValido.MatchString(slug) {
		return nil, fmt.Errorf("%w: slug inválido", domain.ErrBorradorInvalido)
	}
	if tenancy.Reservado(slug) {
		return nil, fmt.Errorf("%w: el slug %q está reservado", domain.ErrConflictoBorrador, slug)
	}
	if ocupado, err := s.slugOcupado(ctx, slug, uuid.Nil); err != nil {
		return nil, err
	} else if ocupado {
		return nil, fmt.Errorf("%w: el slug %q ya está en uso", domain.ErrConflictoBorrador, slug)
	}

	templateKey := strings.ToLower(strings.TrimSpace(input.Template))
	if templateKey == "" {
		templateKey = "producto"
	}

	plantilla, err := plantillas.ObtenerPlantilla(templateKey)
	if err != nil {
		plantilla, err = plantillas.ObtenerPlantilla("producto")
		if err != nil {
			return nil, fmt.Errorf("pages: plantilla base no encontrada: %w", err)
		}
	}

	irData := plantilla.IR
	if len(input.Languages) > 0 {
		irData.Languages.Default = input.Languages[0]
		if len(input.Languages) > 1 {
			irData.Languages.Others = input.Languages[1:]
		} else {
			irData.Languages.Others = []string{}
		}
	}
	if irData.SEO.SiteName == nil {
		irData.SEO.SiteName = make(map[string]string)
	}
	irData.SEO.SiteName[irData.Languages.Default] = nombre

	rawDraft, err := json.Marshal(irData)
	if err != nil {
		return nil, fmt.Errorf("pages: serializar draft: %w", err)
	}

	var sc domain.SitioCompleto
	var rawLegal []byte
	err = s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := comprobarTopeSitiosTx(ctx, tx, tenant); err != nil {
			return err
		}

		legalVacio, _ := json.Marshal(domain.DatosLegales{})
		row := tx.QueryRow(ctx, `
			INSERT INTO sites (
				tenant, slug, name, template, status, draft, draft_rev,
				draft_updated_by, draft_updated_at, legal, analytics_enabled,
				export_origins, export_app_schemes, created_by, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, 'active', $5, 1,
				$6, now(), $7, true,
				'{}', '{}', $6, now(), now()
			) RETURNING id, tenant, slug, name, template, status, suspended_reason, draft, draft_rev,
			            published_number, canonical_host, legal, analytics_enabled, analytics_site_id,
			            export_origins, export_app_schemes, created_at, updated_at, deleted_at`,
			tenant, slug, nombre, templateKey, rawDraft, sub, legalVacio,
		)

		return row.Scan(
			&sc.ID, &sc.Tenant, &sc.Slug, &sc.Name, &sc.Template, &sc.Status, &sc.SuspendedReason,
			&sc.Draft, &sc.DraftRev, &sc.PublishedNumber, &sc.CanonicalHost, &rawLegal,
			&sc.AnalyticsEnabled, &sc.AnalyticsSiteID, &sc.ExportOrigins, &sc.ExportAppSchemes,
			&sc.CreatedAt, &sc.UpdatedAt, &sc.DeletedAt,
		)
	})
	if err != nil {
		return nil, conflictoDeSlug(err)
	}
	if len(rawLegal) > 0 {
		_ = json.Unmarshal(rawLegal, &sc.Legal)
	}
	return &sc, nil
}

// slugOcupado dice si un slug es de otra web: de cualquier sitio de cualquier
// organización —también de uno borrado: el slug de una web borrada no vuelve a quedar
// libre (WEB-08)— o retirado hace menos de un año por otro sitio. Va en modo público,
// el único que ve los de todas las organizaciones también con la RLS evaluada. `sitio`
// es el que se renombra (uuid.Nil en un alta): el slug retirado de sí mismo lo recupera.
func (s *Store) slugOcupado(ctx context.Context, slug string, sitio uuid.UUID) (bool, error) {
	var ocupado bool
	err := s.enPublico(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM sites WHERE slug = $1 AND id <> $2
				UNION
				SELECT 1 FROM retired_slugs WHERE slug = $1 AND until > now() AND (site_id IS NULL OR site_id <> $2)
			)`, slug, sitio).Scan(&ocupado)
	})
	return ocupado, err
}

// conflictoDeSlug traduce la unicidad de `sites.slug`: si dos altas o dos cambios de
// dirección se cruzan, es lo único que una organización ve de los sitios de otra.
func conflictoDeSlug(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "sites_slug_key" {
		return fmt.Errorf("%w: el slug ya está en uso", domain.ErrConflictoBorrador)
	}
	return err
}

// Topes de webs por organización (PAGES-PLAN §17, WEB-08): 20 a la vez, contando las
// borradas que aún se pueden restaurar (si no, restaurar pasaría del tope), y 5 nuevas
// al día. Sin ellos, una organización acaparaba slugs —nombres de marca, los que
// parecen oficiales— sin límite.
const (
	maxSitiosOrganizacion = 20
	maxSitiosNuevosAlDia  = 5
)

// comprobarTopeSitiosTx aplica los topes con un cerrojo por organización: dos altas a
// la vez no pasan las dos el recuento.
func comprobarTopeSitiosTx(ctx context.Context, tx pgx.Tx, tenant string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('pages:alta-sitio:' || $1, 0))`, tenant); err != nil {
		return err
	}
	var vivos, hoy int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE deleted_at IS NULL OR deleted_at >= now() - interval '30 days'),
		       count(*) FILTER (WHERE created_at > now() - interval '1 day')
		  FROM sites
		 WHERE tenant = $1`, tenant).Scan(&vivos, &hoy); err != nil {
		return err
	}
	if vivos >= maxSitiosOrganizacion || hoy >= maxSitiosNuevosAlDia {
		return domain.ErrLimiteSitios
	}
	return nil
}

func resolverNombreSitio(actual string, in *string) (string, error) {
	if in == nil {
		return actual, nil
	}
	trimmed := strings.TrimSpace(*in)
	if len(trimmed) < 1 || len(trimmed) > 120 {
		return "", fmt.Errorf("%w: nombre inválido (1-120 caracteres)", domain.ErrBorradorInvalido)
	}
	return trimmed, nil
}

func (s *Store) resolverSlugSitio(ctx context.Context, tx pgx.Tx, tenant string, siteID uuid.UUID, actual string, in *string) (string, error) {
	if in == nil {
		return actual, nil
	}
	trimmedSlug := strings.ToLower(strings.TrimSpace(*in))
	if !reSlugValido.MatchString(trimmedSlug) {
		return "", fmt.Errorf("%w: slug inválido", domain.ErrBorradorInvalido)
	}
	if trimmedSlug == actual {
		return actual, nil
	}
	if tenancy.Reservado(trimmedSlug) {
		return "", fmt.Errorf("%w: el slug %q está reservado", domain.ErrConflictoBorrador, trimmedSlug)
	}
	existe, err := s.slugOcupado(ctx, trimmedSlug, siteID)
	if err != nil {
		return "", err
	}
	if existe {
		return "", fmt.Errorf("%w: el slug %q ya está en uso", domain.ErrConflictoBorrador, trimmedSlug)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO retired_slugs (slug, tenant, site_id, until)
		VALUES ($1, $2, $3, now() + interval '1 year')
		ON CONFLICT (slug) DO UPDATE SET until = EXCLUDED.until, site_id = EXCLUDED.site_id`,
		actual, tenant, siteID,
	)
	if err != nil {
		return "", fmt.Errorf("pages: registrar slug retirado: %w", err)
	}
	return trimmedSlug, nil
}

// ActualizarAjustesSitio actualiza los metadatos y ajustes de un sitio (nombre, slug, legal, analítica, exportación).
//
// `administra` dice si quien lo pide es `owner` o `admin` de la organización. Sin serlo no
// se cambia la DIRECCIÓN del sitio (el slug) ni sus DATOS LEGALES, que son lo que la
// organización firma en público: ErrSoloAdministracion (WEB-02). Se decide aquí y no en el
// manejador porque es aquí donde se sabe si el slug que llega es otro que el que hay.
func (s *Store) ActualizarAjustesSitio(ctx context.Context, tenant string, siteID uuid.UUID, in domain.ActualizarSitioInput, administra bool) (*domain.SitioCompleto, error) {
	if tenant == "" {
		return nil, domain.ErrNoExiste
	}

	var sc domain.SitioCompleto
	var rawLegal []byte
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var slugActual, nameActual string
		err := tx.QueryRow(ctx, `SELECT slug, name FROM sites WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`, siteID, tenant).
			Scan(&slugActual, &nameActual)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoExiste
		}
		if err != nil {
			return err
		}

		nuevoNombre, err := resolverNombreSitio(nameActual, in.Name)
		if err != nil {
			return err
		}
		cambiaSlug := in.Slug != nil && strings.ToLower(strings.TrimSpace(*in.Slug)) != slugActual
		if !administra && (cambiaSlug || in.Legal != nil) {
			return domain.ErrSoloAdministracion
		}

		nuevoSlug, err := s.resolverSlugSitio(ctx, tx, tenant, siteID, slugActual, in.Slug)
		if err != nil {
			return err
		}

		var legalBytes []byte
		if in.Legal != nil {
			legalBytes, err = json.Marshal(in.Legal)
			if err != nil {
				return err
			}
		}

		// `canonical_host` y `analytics_site_id` NO se escriben aquí: no son ajustes de
		// quien edita, los fija el servidor (FijarCanonico, FijarAnaliticaSitio).
		row := tx.QueryRow(ctx, `
			UPDATE sites
			   SET name = $1,
			       slug = $2,
			       legal = CASE WHEN $3::jsonb IS NOT NULL THEN $3::jsonb ELSE legal END,
			       analytics_enabled = COALESCE($4, analytics_enabled),
			       export_origins = COALESCE($5, export_origins),
			       export_app_schemes = COALESCE($6, export_app_schemes),
			       updated_at = now()
			 WHERE id = $7 AND tenant = $8 AND deleted_at IS NULL
			 RETURNING id, tenant, slug, name, template, status, suspended_reason, draft, draft_rev,
			           published_number, canonical_host, legal, analytics_enabled, analytics_site_id,
			           export_origins, export_app_schemes, created_at, updated_at, deleted_at`,
			nuevoNombre, nuevoSlug, legalBytes, in.AnalyticsEnabled,
			in.ExportOrigins, in.ExportAppSchemes, siteID, tenant,
		)

		return row.Scan(
			&sc.ID, &sc.Tenant, &sc.Slug, &sc.Name, &sc.Template, &sc.Status, &sc.SuspendedReason,
			&sc.Draft, &sc.DraftRev, &sc.PublishedNumber, &sc.CanonicalHost, &rawLegal,
			&sc.AnalyticsEnabled, &sc.AnalyticsSiteID, &sc.ExportOrigins, &sc.ExportAppSchemes,
			&sc.CreatedAt, &sc.UpdatedAt, &sc.DeletedAt,
		)
	})
	if err != nil {
		return nil, conflictoDeSlug(err)
	}
	if len(rawLegal) > 0 {
		_ = json.Unmarshal(rawLegal, &sc.Legal)
	}
	return &sc, nil
}

// FijarCanonico pone la dirección principal de un sitio. Solo admite un dominio ACTIVO de
// ese mismo sitio —el que ya pasó la verificación TXT, el enrutado y la prueba HTTPS— o
// "", que la devuelve a `<slug>.<PAGES_SITES_DOMAIN>`. Es la ÚNICA función que escribe
// `canonical_host` con un valor: fue texto libre del navegador y un sitio se quedaba con
// el host de otro escribiéndolo ahí (WEB-01).
func (s *Store) FijarCanonico(ctx context.Context, tenant string, siteID uuid.UUID, host string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE sites
			   SET canonical_host = NULLIF($3, ''), updated_at = now()
			 WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL
			   AND ($3 = '' OR EXISTS (
			         SELECT 1 FROM domains d
			          WHERE d.site_id = sites.id AND d.tenant = sites.tenant
			            AND d.host = $3 AND d.status = 'active'))`,
			siteID, tenant, host)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return nil
	})
}

// FijarAnaliticaSitio guarda el identificador que Sytue Analytics dio al host del sitio.
// Lo llama la publicación, nunca el navegador.
func (s *Store) FijarAnaliticaSitio(ctx context.Context, tenant string, siteID uuid.UUID, analyticsID string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE sites SET analytics_site_id = $3, updated_at = now() WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`,
			siteID, tenant, analyticsID)
		return err
	})
}

// soltarCanonicos quita el canónico de los sitios cuyo dominio ha dejado de estar activo.
// Va en la MISMA transacción que degrada o borra el dominio: un canónico sin dominio
// activo detrás no puede existir ni un instante.
func soltarCanonicos(ctx context.Context, tx pgx.Tx, tenant string, siteID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE sites
		   SET canonical_host = NULL, updated_at = now()
		 WHERE id = $1 AND tenant = $2 AND canonical_host IS NOT NULL
		   AND NOT EXISTS (
		         SELECT 1 FROM domains d
		          WHERE d.site_id = sites.id AND d.tenant = sites.tenant
		            AND d.host = sites.canonical_host AND d.status = 'active')`,
		siteID, tenant)
	return err
}

// BorrarSitio aplica un borrado blando marcando deleted_at = now().
func (s *Store) BorrarSitio(ctx context.Context, tenant string, id uuid.UUID) error {
	if tenant == "" {
		return domain.ErrNoExiste
	}
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE sites SET deleted_at = now(), updated_at = now() WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`, id, tenant)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		// Invalida dominios verificados/activos del sitio liberando el host (§8.3)
		if _, err := tx.Exec(ctx, `UPDATE domains SET status = 'failed', detail = 'site_deleted', updated_at = now() WHERE site_id = $1 AND tenant = $2 AND status IN ('verified', 'active')`, id, tenant); err != nil {
			return err
		}
		return soltarCanonicos(ctx, tx, tenant, id)
	})
}

// RestaurarSitio recupera un sitio borrado si está dentro del plazo de 30 días.
func (s *Store) RestaurarSitio(ctx context.Context, tenant string, id uuid.UUID) error {
	if tenant == "" {
		return domain.ErrNoExiste
	}
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var deletedAt *time.Time
		err := tx.QueryRow(ctx, `SELECT deleted_at FROM sites WHERE id = $1 AND tenant = $2`, id, tenant).Scan(&deletedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoExiste
		}
		if err != nil {
			return err
		}
		if deletedAt == nil {
			return nil
		}
		if time.Since(*deletedAt) > 30*24*time.Hour {
			return errors.New("pages: el sitio superó el plazo de restauración de 30 días")
		}

		_, err = tx.Exec(ctx, `UPDATE sites SET deleted_at = NULL, updated_at = now() WHERE id = $1 AND tenant = $2`, id, tenant)
		return err
	})
}

// ListarVersiones devuelve las versiones de diseño guardadas para un sitio.
func (s *Store) ListarVersiones(ctx context.Context, tenant string, siteID uuid.UUID) ([]domain.VersionSitio, error) {
	if tenant == "" {
		return nil, domain.ErrNoExiste
	}
	var versiones []domain.VersionSitio
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, name, kind, draft_rev, created_by, created_at
			  FROM site_versions
			 WHERE site_id = $1 AND tenant = $2
			 ORDER BY created_at DESC`, siteID, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v domain.VersionSitio
			if err := rows.Scan(&v.ID, &v.Name, &v.Kind, &v.DraftRev, &v.CreatedBy, &v.CreatedAt); err != nil {
				return err
			}
			versiones = append(versiones, v)
		}
		return rows.Err()
	})
	return versiones, err
}

// RestaurarVersion restaura el borrador de un sitio al IR de una versión guardada.
func (s *Store) RestaurarVersion(ctx context.Context, tenant string, siteID, versionID uuid.UUID, sub string) (int64, error) {
	if tenant == "" || sub == "" {
		return 0, domain.ErrNoExiste
	}
	var nuevaRev int64
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var versionIR []byte
		var revActual int64
		err := tx.QueryRow(ctx, `
			SELECT ir FROM site_versions WHERE id = $1 AND site_id = $2 AND tenant = $3`,
			versionID, siteID, tenant,
		).Scan(&versionIR)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoExiste
		}
		if err != nil {
			return err
		}

		var currentDraft []byte
		err = tx.QueryRow(ctx, `
			SELECT draft, draft_rev FROM sites WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`,
			siteID, tenant,
		).Scan(&currentDraft, &revActual)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO site_versions (tenant, site_id, name, kind, ir, draft_rev, created_by)
			VALUES ($1, $2, 'Antes de restaurar', 'before_restore', $3, $4, $5)`,
			tenant, siteID, currentDraft, revActual, sub,
		)
		if err != nil {
			return err
		}

		err = tx.QueryRow(ctx, `
			UPDATE sites
			   SET draft = $1,
			       draft_rev = draft_rev + 1,
			       draft_updated_by = $2,
			       draft_updated_at = now(),
			       updated_at = now()
			 WHERE id = $3 AND tenant = $4 AND deleted_at IS NULL
			 RETURNING draft_rev`,
			versionIR, sub, siteID, tenant,
		).Scan(&nuevaRev)
		return err
	})
	return nuevaRev, err
}

// ComprobarSlugDisponible comprueba si un slug está libre para su uso.
func (s *Store) ComprobarSlugDisponible(ctx context.Context, slug string) (bool, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !reSlugValido.MatchString(slug) || tenancy.Reservado(slug) {
		return false, nil
	}
	ocupado, err := s.slugOcupado(ctx, slug, uuid.Nil)
	return !ocupado, err
}

// BuscarSitios devuelve los sitios vivos de una organización cuyo nombre o slug contiene
// q (sin distinguir mayúsculas), del editado más recientemente al que más tiempo lleva
// quieto. q vacío = los más recientes: es la «actividad reciente» del Hub.
//
// Lee solo lo que enseña un resultado (no el borrador, que pesa hasta 2 MiB), y compara
// con strpos y no con LIKE: el término es de la persona y `%` o `_` no deben comodinear.
// ponytail: sin acentos plegados ni índice; una organización tiene decenas de sitios,
// no millones. Si llegaran a miles, un índice trigram sobre lower(name).
func (s *Store) BuscarSitios(ctx context.Context, tenant, q string, limite int) ([]domain.ResultadoBusqueda, error) {
	if limite <= 0 || limite > 50 {
		limite = 20
	}
	q = strings.ToLower(strings.TrimSpace(q))
	var out []domain.ResultadoBusqueda
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, slug, name, published_number, updated_at
			  FROM sites
			 WHERE tenant = $1 AND deleted_at IS NULL
			   AND ($2 = '' OR strpos(lower(name), $2) > 0 OR strpos(slug, $2) > 0)
			 ORDER BY updated_at DESC
			 LIMIT $3`, tenant, q, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r domain.ResultadoBusqueda
			if err := rows.Scan(&r.ID, &r.Slug, &r.Name, &r.PublishedNumber, &r.UpdatedAt); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}
