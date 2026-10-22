// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// SetFavorite marca (on=true) o desmarca (on=false) un contacto como destacado.
// El contacto se direcciona por su UID (lo que el frontend obtiene de CardDAV) y
// SOLO se acepta si pertenece a una libreta del propio dueno (subject): un
// usuario no puede destacar contactos de otro aunque compartan tenant. Devuelve
// ErrNotFound si no hay contacto vivo del dueno con ese UID.
func (s *Store) SetFavorite(ctx context.Context, subject, uid string, on bool) error {
	t := tenancy.MustFrom(ctx)
	// El frontend obtiene el UID de la vCard (CardDAV), que puede llevar el
	// prefijo urn:uuid:; contacts.uid lo guarda desnudo. Normalizamos para casar.
	uid = domain.CleanUID(strings.TrimSpace(uid))
	if uid == "" {
		return domain.ErrNotFound
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE contacts SET is_favorite=$4, updated_at=now()
			 WHERE tenant=$1 AND uid=$3 AND deleted_at IS NULL
			   AND address_book_id IN (
			       SELECT id FROM address_books
			       WHERE tenant=$1 AND owner_sub=$2 AND deleted_at IS NULL)`,
			t.Slug, subject, uid, on)
		if err != nil {
			return fmt.Errorf("store: marcar favorito: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ListFavoriteUIDs devuelve los UID de los contactos vivos destacados del dueno
// (todas sus libretas), en orden por nombre. El frontend cruza este conjunto con
// su lista CardDAV para pintar la estrella y filtrar «Favoritos». El borrado
// suave los oculta (deleted_at IS NULL) y restaurar los revive.
func (s *Store) ListFavoriteUIDs(ctx context.Context, subject string) ([]string, error) {
	t := tenancy.MustFrom(ctx)
	var out []string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT c.uid FROM contacts c
			 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
			 WHERE c.tenant=$1 AND ab.owner_sub=$2 AND c.is_favorite AND c.deleted_at IS NULL
			 ORDER BY lower(c.formatted_name)`,
			t.Slug, subject)
		if err != nil {
			return fmt.Errorf("store: listar favoritos: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var uid string
			if err := rows.Scan(&uid); err != nil {
				return err
			}
			out = append(out, uid)
		}
		return rows.Err()
	})
	return out, err
}
