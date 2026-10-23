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

// ContactCursor contains the database's sort keys, not Go's approximation of SQL lower().
type ContactCursor struct {
	Family string    `json:"family"`
	Given  string    `json:"given"`
	Name   string    `json:"name"`
	ID     uuid.UUID `json:"id"`
}

// ContactPage is a bounded page with an optional continuation.
type ContactPage struct {
	Contacts []domain.Contact
	Next     *ContactCursor
}

type contactCursorRow struct {
	pgx.Row
	cursor *ContactCursor
}

func (r contactCursorRow) Scan(dest ...any) error {
	return r.Row.Scan(append(dest, &r.cursor.Family, &r.cursor.Given, &r.cursor.Name)...)
}

// ListContactsPage retains the same tenant and owner checks as ListContacts on EVERY
// page. Keyset order is identical to the legacy route; a deleted previous contact
// does not invalidate the continuation. Channels are attached in one batch.
func (s *Store) ListContactsPage(ctx context.Context, subject string, bookID uuid.UUID, after *ContactCursor, limit int) (ContactPage, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	page := ContactPage{Contacts: []domain.Contact{}}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := assertOwnedBook(ctx, tx, t.Slug, subject, bookID); err != nil {
			return err
		}
		var position ContactCursor
		if after != nil {
			position = *after
		}
		rows, err := tx.Query(ctx, `SELECT `+contactCols+`, lower(c.family_name), lower(c.given_name), lower(c.formatted_name)
   FROM contacts c JOIN card_objects o ON o.id=c.object_id AND o.tenant=c.tenant
   WHERE c.tenant=$1 AND c.address_book_id=$2 AND c.deleted_at IS NULL
    AND ($3::boolean OR (lower(c.family_name),lower(c.given_name),lower(c.formatted_name),c.id)>($4,$5,$6,$7::uuid))
   ORDER BY lower(c.family_name),lower(c.given_name),lower(c.formatted_name),c.id LIMIT $8`,
			t.Slug, bookID, after == nil, position.Family, position.Given, position.Name, position.ID, limit+1)
		if err != nil {
			return fmt.Errorf("store: pagina contactos: %w", err)
		}
		var last ContactCursor
		for rows.Next() {
			var cursor ContactCursor
			c, err := scanContact(contactCursorRow{Row: rows, cursor: &cursor}, t.Slug)
			if err != nil {
				rows.Close()
				return err
			}
			cursor.ID = c.ID
			if len(page.Contacts) == limit {
				page.Next = &last
				break
			}
			page.Contacts = append(page.Contacts, c)
			last = cursor
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		return attachChannels(ctx, tx, t.Slug, page.Contacts)
	})
	return page, err
}
