// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Retos de validación de dominio (DCV): el fichero que una CA pública pide ver
// en `http://<dominio>/.well-known/pki-validation/<nombre>` para creerse que el
// dominio es nuestro. Ver la migración 0061.

// GuardarRetoDeDominio deja listo un reto para que el handler público lo sirva.
//
// El nombre y el contenido los pone la CA y se guardan TAL CUAL: cada emisor usa
// su formato, e interpretarlo sería atarnos a uno.
//
// Sustituye el que hubiera para ese dominio y nombre: reintentar una emisión que
// falló a medias es lo normal, y dejar el viejo haría que la CA leyera el
// contenido de la tentativa anterior.
func (s *Store) GuardarRetoDeDominio(ctx context.Context, orgID, dominio, nombre, contenido string, caduca time.Time) error {
	_, err := s.DB().Exec(ctx, `
		INSERT INTO domain_validation_challenges (org_id, domain, name, content, expires_at)
		VALUES ($1, lower($2), $3, $4, $5)
		ON CONFLICT (domain, name) DO UPDATE
		SET content = EXCLUDED.content, expires_at = EXCLUDED.expires_at, org_id = EXCLUDED.org_id`,
		orgID, strings.TrimSpace(dominio), nombre, contenido, caduca)
	return errf("GuardarRetoDeDominio", err)
}

// RetoDeDominio devuelve el contenido a servir, o cadena vacía si no hay reto
// vivo. No distingue «no existe» de «caducado»: hacia fuera son lo mismo y
// contarlo sería decir que ahí hubo una emisión.
func (s *Store) RetoDeDominio(ctx context.Context, dominio, nombre string) (string, error) {
	var contenido string
	err := s.DB().QueryRow(ctx, `
		SELECT content FROM domain_validation_challenges
		WHERE domain = lower($1) AND name = $2 AND expires_at > now()`,
		strings.TrimSpace(dominio), nombre).Scan(&contenido)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", errf("RetoDeDominio", err)
	}
	return contenido, nil
}

// PurgarRetosDeDominio borra los caducados. Un reto vencido no sirve para nada y
// sigue siendo un fichero que acredita control del dominio.
func (s *Store) PurgarRetosDeDominio(ctx context.Context) (int64, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM domain_validation_challenges WHERE expires_at <= now()`)
	if err != nil {
		return 0, errf("PurgarRetosDeDominio", err)
	}
	return tag.RowsAffected(), nil
}
