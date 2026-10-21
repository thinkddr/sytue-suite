// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// ContactEmailSet devuelve el conjunto (en minusculas) de los correos de los
// contactos VIVOS de una libreta. La importacion CSV lo usa para deduplicar
// contra lo ya existente (por correo), igual que la fusion de duplicados.
func (s *Store) ContactEmailSet(ctx context.Context, bookID uuid.UUID) (map[string]struct{}, error) {
	t := tenancy.MustFrom(ctx)
	set := make(map[string]struct{})
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT lower(e.email) FROM contact_emails e
			 JOIN contacts c ON c.id = e.contact_id AND c.tenant = e.tenant
			 WHERE e.tenant=$1 AND c.address_book_id=$2 AND c.deleted_at IS NULL`,
			t.Slug, bookID)
		if err != nil {
			return fmt.Errorf("store: leer correos existentes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var email string
			if err := rows.Scan(&email); err != nil {
				return err
			}
			set[email] = struct{}{}
		}
		return rows.Err()
	})
	return set, err
}

// ListOwnerContacts devuelve TODOS los contactos vivos de un dueno (todas sus
// libretas), decodificados de su vCard canonica para incluir tambien las
// DIRECCIONES (que la proyeccion normalizada no almacena en columnas). Es la
// fuente del export CSV: reutiliza el mismo mapeo del round-trip vCard.
func (s *Store) ListOwnerContacts(ctx context.Context, subject string) ([]domain.Contact, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Contact
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT o.data FROM card_objects o
			 JOIN contacts c ON c.object_id = o.id AND c.tenant = o.tenant
			 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
			 WHERE o.tenant=$1 AND ab.owner_sub=$2 AND c.deleted_at IS NULL AND o.deleted_at IS NULL
			 ORDER BY lower(c.family_name), lower(c.given_name), lower(c.formatted_name)`,
			t.Slug, subject)
		if err != nil {
			return fmt.Errorf("store: leer tarjetas del dueno: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var data string
			if err := rows.Scan(&data); err != nil {
				return err
			}
			c, derr := domain.DecodeContact(data)
			if derr != nil {
				continue // tarjeta corrupta: se omite del export sin abortar
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}
