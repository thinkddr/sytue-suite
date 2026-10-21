// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// GetUnfurl devuelve la vista previa cacheada de una URL (por tenant), y found=false
// si no hay fila. El hash (sha256 hex) es la clave: el índice de la PK (tenant,
// url_hash) lo cubre sin escaneo. El servicio decide si la fila está fresca según
// su TTL (fetched_at).
func (s *Store) GetUnfurl(ctx context.Context, url string) (domain.LinkUnfurl, bool, error) {
	t := tenancy.MustFrom(ctx)
	hash := domain.HashToken(url)
	var u domain.LinkUnfurl
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT url, title, description, image_url, site_name, fetched_at
			 FROM link_unfurls WHERE tenant=$1 AND url_hash=$2`,
			t.Slug, hash).Scan(&u.URL, &u.Title, &u.Description, &u.ImageURL, &u.SiteName, &u.FetchedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LinkUnfurl{}, false, nil
	}
	if err != nil {
		return domain.LinkUnfurl{}, false, err
	}
	return u, true, nil
}

// GetUnfurlByHash busca la vista previa por el hash de su URL, sin conocerla.
//
// Existe para el proxy de la imagen: el cliente pide una miniatura por hash, y
// esta consulta es la LISTA BLANCA que impide que el servicio se convierta en un
// proxy abierto de recursos ajenos. Solo devuelve fila si alguien de ESTE tenant
// pegó ese enlace y se cacheó su vista previa; para cualquier otro hash no hay
// nada que servir, así que no hay forma de pedirle al servidor que descargue una
// URL arbitraria.
//
// El aislamiento por tenant lo impone la misma cláusula que en GetUnfurl: dos
// organizaciones que hayan pegado el mismo enlace tienen filas distintas.
func (s *Store) GetUnfurlByHash(ctx context.Context, hash string) (domain.LinkUnfurl, bool, error) {
	t := tenancy.MustFrom(ctx)
	var u domain.LinkUnfurl
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT url, title, description, image_url, site_name, fetched_at
			 FROM link_unfurls WHERE tenant=$1 AND url_hash=$2`,
			t.Slug, hash).Scan(&u.URL, &u.Title, &u.Description, &u.ImageURL, &u.SiteName, &u.FetchedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LinkUnfurl{}, false, nil
	}
	if err != nil {
		return domain.LinkUnfurl{}, false, err
	}
	return u, true, nil
}

// UpsertUnfurl guarda (o refresca) la vista previa cacheada de una URL para el
// tenant. Una fila con campos vacíos es un resultado NEGATIVO válido (la URL no dio
// metadatos): evita re-fetchearla dentro del TTL. fetched_at se pone a now() en cada
// upsert.
func (s *Store) UpsertUnfurl(ctx context.Context, u domain.LinkUnfurl) error {
	t := tenancy.MustFrom(ctx)
	hash := domain.HashToken(u.URL)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO link_unfurls (tenant, url_hash, url, title, description, image_url, site_name, fetched_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, now())
			 ON CONFLICT (tenant, url_hash) DO UPDATE SET
			   url=EXCLUDED.url, title=EXCLUDED.title, description=EXCLUDED.description,
			   image_url=EXCLUDED.image_url, site_name=EXCLUDED.site_name, fetched_at=now()`,
			t.Slug, hash, u.URL, u.Title, u.Description, u.ImageURL, u.SiteName)
		return err
	})
}
