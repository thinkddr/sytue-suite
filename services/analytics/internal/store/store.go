// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store es el acceso a PostgreSQL de Sytue Analytics. Tres caminos, y
// cada uno abre solo lo suyo (ADR 0026 §4):
//
//	InRLSTx          la API de lectura: app.tenant, aislamiento por inquilino.
//	inIngestTx       el colector: app.ingest, INSERT en eventos y SELECT de
//	                 sitios y sal. NO puede leer eventos.
//	InMaintenanceTx  el rollup y la purga, cross-tenant por naturaleza.
//
// Ningún rol con BYPASSRLS en ningún caso.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// IngestSettingKey es la variable de sesión que habilita las políticas del
// colector (INSERT de eventos, SELECT de sitios y de la sal).
const IngestSettingKey = "app.ingest"

// ErrNotFound lo devuelven las lecturas cuando no hay fila visible: puede ser
// que no exista o que sea de otra organización, y NO se distingue.
var ErrNotFound = errors.New("analytics: no encontrado")

// ErrHostDeOtro: el hostname ya lo mide otra organización. Un host existente no
// cambia de manos, lo pida quien lo pida.
var ErrHostDeOtro = errors.New("analytics: ese hostname ya lo mide otra organización")

// organizacion es la del contexto, para ponerla EN EL SQL de cada sentencia.
//
// Las políticas de fila de estas tablas dicen lo mismo, pero una política no se
// evalúa para un superusuario ni para un rol con BYPASSRLS, y el despliegue ha
// llegado a conectar así: mientras el filtro vivía solo en la política, `ListSites`
// devolvía los sitios de todas las organizaciones y `DeleteSite` borraba el de
// cualquiera. El predicado explícito vale con cualquier rol; la política queda
// como segunda barrera.
func organizacion(ctx context.Context) (string, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return "", fmt.Errorf("analytics: sin organización en el contexto: %w", err)
	}
	return t.Slug, nil
}

// Store agrupa el acceso a datos de Sytue Analytics.
type Store struct {
	pool *pg.Pool
}

// New construye el store sobre un pool ya conectado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// inIngestTx abre una transacción con app.ingest='on' (SET LOCAL). Es el único
// permiso que tiene el proceso de ingestión, y deliberadamente no incluye
// app.maintenance: con él podría LEER eventos de todos los inquilinos.
func (s *Store) inIngestTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('"+IngestSettingKey+"', 'on', true)"); err != nil {
			return fmt.Errorf("analytics: set_config app.ingest: %w", err)
		}
		return fn(tx)
	})
}

// Site es un sitio medido. Un sitio ES un hostname: el colector lo resuelve por
// la cabecera Host, así que no hay identificador que falsificar para inflar los
// números de otro cliente.
type Site struct {
	ID             string    `json:"id"`
	Host           string    `json:"host"`
	EventAllowlist []string  `json:"event_allowlist"`
	CreatedAt      time.Time `json:"created_at"`
	// Tenant no se serializa: el cliente ya sabe de qué organización es y
	// publicarlo solo abriría una vía de enumeración.
	Tenant string `json:"-"`
}

// SiteByHost resuelve el hostname de la cabecera Host. Corre bajo app.ingest
// porque el colector aún no sabe de qué inquilino es la petición — es la única
// consulta cross-tenant que puede hacer, y es de SOLO LECTURA sobre sitios.
func (s *Store) SiteByHost(ctx context.Context, host string) (Site, error) {
	var site Site
	err := s.inIngestTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id::text, tenant, host, event_allowlist, created_at
			   FROM analytics_sites WHERE host = $1`, host).
			Scan(&site.ID, &site.Tenant, &site.Host, &site.EventAllowlist, &site.CreatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Site{}, ErrNotFound
	}
	if err != nil {
		return Site{}, fmt.Errorf("analytics: resolver sitio %q: %w", host, err)
	}
	return site, nil
}

// ListSites devuelve los sitios de la organización del contexto.
func (s *Store) ListSites(ctx context.Context) ([]Site, error) {
	org, err := organizacion(ctx)
	if err != nil {
		return nil, err
	}
	var out []Site
	err = s.pool.InRLSTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id::text, tenant, host, event_allowlist, created_at
			   FROM analytics_sites WHERE tenant = $1 ORDER BY host`, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var st Site
			if err := rows.Scan(&st.ID, &st.Tenant, &st.Host, &st.EventAllowlist, &st.CreatedAt); err != nil {
				return err
			}
			out = append(out, st)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("analytics: listar sitios: %w", err)
	}
	return out, nil
}

// CreateSite da de alta un sitio para el inquilino del contexto. El host es
// UNIQUE global: si otro inquilino ya lo registró, falla — y eso es justo lo que
// impide medir el dominio de otro.
func (s *Store) CreateSite(ctx context.Context, host string, allowlist []string) (Site, error) {
	if allowlist == nil {
		allowlist = []string{}
	}
	org, err := organizacion(ctx)
	if err != nil {
		return Site{}, err
	}
	var site Site
	err = s.pool.InRLSTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO analytics_sites (tenant, host, event_allowlist)
			 VALUES ($1, $2, $3)
			 RETURNING id::text, tenant, host, event_allowlist, created_at`,
			org, host, allowlist).
			Scan(&site.ID, &site.Tenant, &site.Host, &site.EventAllowlist, &site.CreatedAt)
	})
	if err != nil {
		return Site{}, fmt.Errorf("analytics: crear sitio %q: %w", host, err)
	}
	return site, nil
}

// EnsureSite da de alta un host reservado para la organización que afirma Pages,
// o le actualiza la lista de eventos si ya es suyo. Solo se invoca tras la
// frontera de secreto interno.
//
// UN HOST QUE YA EXISTE NO CAMBIA DE ORGANIZACIÓN. Antes el conflicto reescribía
// también `tenant`: bastaba pedir el alta de un hostname ajeno para quedarse el
// sitio, con su identificador y su historial detrás. Si el host es de otra,
// ErrHostDeOtro: quien lo libera es su dueña, borrando el sitio.
func (s *Store) EnsureSite(ctx context.Context, host string, allowlist []string) (Site, error) {
	if allowlist == nil {
		allowlist = []string{}
	}
	org, err := organizacion(ctx)
	if err != nil {
		return Site{}, err
	}
	var site Site
	err = s.pool.InRLSTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.pages_hosts', 'on', true)"); err != nil {
			return fmt.Errorf("analytics: habilitar alta interna de Pages: %w", err)
		}
		return tx.QueryRow(ctx,
			`INSERT INTO analytics_sites (tenant, host, event_allowlist)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (host) DO UPDATE
			 SET event_allowlist = EXCLUDED.event_allowlist
			 WHERE analytics_sites.tenant = EXCLUDED.tenant
			 RETURNING id::text, tenant, host, event_allowlist, created_at`, org, host, allowlist).
			Scan(&site.ID, &site.Tenant, &site.Host, &site.EventAllowlist, &site.CreatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Site{}, ErrHostDeOtro
	}
	if err != nil {
		return Site{}, fmt.Errorf("analytics: asegurar sitio %q: %w", host, err)
	}
	return site, nil
}

// UpdateAllowlist reemplaza la allowlist de nombres de evento del sitio.
func (s *Store) UpdateAllowlist(ctx context.Context, id string, allowlist []string) (Site, error) {
	if allowlist == nil {
		allowlist = []string{}
	}
	org, err := organizacion(ctx)
	if err != nil {
		return Site{}, err
	}
	var site Site
	err = s.pool.InRLSTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`UPDATE analytics_sites SET event_allowlist = $2 WHERE id = $1 AND tenant = $3
			 RETURNING id::text, tenant, host, event_allowlist, created_at`, id, allowlist, org).
			Scan(&site.ID, &site.Tenant, &site.Host, &site.EventAllowlist, &site.CreatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Site{}, ErrNotFound
	}
	if err != nil {
		return Site{}, fmt.Errorf("analytics: actualizar sitio %s: %w", id, err)
	}
	return site, nil
}

// DeleteSite retira el sitio y, en cascada, todos sus agregados.
func (s *Store) DeleteSite(ctx context.Context, id string) error {
	org, err := organizacion(ctx)
	if err != nil {
		return err
	}
	return s.pool.InRLSTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM analytics_sites WHERE id = $1 AND tenant = $2`, id, org)
		if err != nil {
			return fmt.Errorf("analytics: borrar sitio %s: %w", id, err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}
