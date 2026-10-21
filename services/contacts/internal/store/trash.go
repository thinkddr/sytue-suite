// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// TrashItem es la entrada minima de la papelera (una tarjeta borrada suave), con
// lo justo para listarla y ofrecer restaurar/eliminar: no se decodifica la vCard
// entera porque la papelera solo se lista, no se edita.
type TrashItem struct {
	ID            uuid.UUID `json:"id"`
	UID           string    `json:"uid"`
	FormattedName string    `json:"formatted_name"`
	Organization  string    `json:"organization,omitempty"`
	PrimaryEmail  string    `json:"primary_email,omitempty"`
	DeletedAt     time.Time `json:"deleted_at"`
}

// ListTrash lista los contactos borrados suave del dueno (todas sus libretas),
// reciente-borrado primero (keyset por deleted_at DESC, id DESC). SOLO del
// dueno: el JOIN a address_books filtra por owner_sub. limit acota la pagina.
func (s *Store) ListTrash(ctx context.Context, subject string, limit int) ([]TrashItem, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var out []TrashItem
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT c.id, c.uid, c.formatted_name, c.organization,
			        COALESCE((SELECT e.email FROM contact_emails e
			                  WHERE e.tenant = c.tenant AND e.contact_id = c.id
			                  ORDER BY e.is_primary DESC, e.position LIMIT 1), '') AS primary_email,
			        c.deleted_at
			 FROM contacts c
			 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
			 WHERE c.tenant=$1 AND ab.owner_sub=$2 AND c.deleted_at IS NOT NULL
			 ORDER BY c.deleted_at DESC, c.id DESC
			 LIMIT $3`,
			t.Slug, subject, limit)
		if err != nil {
			return fmt.Errorf("store: listar papelera: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var it TrashItem
			if err := rows.Scan(&it.ID, &it.UID, &it.FormattedName, &it.Organization,
				&it.PrimaryEmail, &it.DeletedAt); err != nil {
				return err
			}
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}

// RestoreContact revive un contacto borrado suave del dueno: limpia deleted_at
// del contacto y de su tarjeta, y avanza el ctag de la libreta. Owner-scoped
// (solo restaura lo propio). Si al revivir colisiona con un UID/recurso ya vivo
// devuelve ErrConflict; si no hay tarjeta borrada del dueno con ese id,
// ErrNotFound.
func (s *Store) RestoreContact(ctx context.Context, subject string, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var objectID, abookID uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT c.object_id, c.address_book_id FROM contacts c
			 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
			 WHERE c.tenant=$1 AND c.id=$2 AND c.deleted_at IS NOT NULL AND ab.owner_sub=$3`,
			t.Slug, id, subject).Scan(&objectID, &abookID)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: leer contacto a restaurar: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE card_objects SET deleted_at=NULL, updated_at=now() WHERE tenant=$1 AND id=$2`,
			t.Slug, objectID); err != nil {
			if uniqueViolation(err) {
				return domain.ErrConflict
			}
			return fmt.Errorf("store: restaurar tarjeta: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE contacts SET deleted_at=NULL, updated_at=now() WHERE tenant=$1 AND id=$2`,
			t.Slug, id); err != nil {
			if uniqueViolation(err) {
				return domain.ErrConflict
			}
			return fmt.Errorf("store: restaurar contacto: %w", err)
		}
		return bumpAddressBook(ctx, tx, t.Slug, abookID)
	})
}

// PurgeContact elimina DEFINITIVAMENTE un contacto de la papelera del dueno
// (borrado en duro). Solo actua sobre lo YA borrado suave (no se puede purgar un
// contacto vivo por esta via) y solo del dueno. Al borrar la tarjeta, la FK ON
// DELETE CASCADE arrastra el contacto y sus correos/telefonos.
func (s *Store) PurgeContact(ctx context.Context, subject string, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var objectID, abookID uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT c.object_id, c.address_book_id FROM contacts c
			 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
			 WHERE c.tenant=$1 AND c.id=$2 AND c.deleted_at IS NOT NULL AND ab.owner_sub=$3`,
			t.Slug, id, subject).Scan(&objectID, &abookID)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: leer contacto a purgar: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM card_objects WHERE tenant=$1 AND id=$2`, t.Slug, objectID); err != nil {
			return fmt.Errorf("store: purgar tarjeta: %w", err)
		}
		return bumpAddressBook(ctx, tx, t.Slug, abookID)
	})
}
