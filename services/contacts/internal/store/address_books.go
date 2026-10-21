// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

const abookCols = `id, owner_sub, name, display_name, description, color,
	is_default, ctag, sync_token, created_at, updated_at`

func scanAddressBook(row pgx.Row, t string) (domain.AddressBook, error) {
	var a domain.AddressBook
	if err := row.Scan(&a.ID, &a.OwnerSub, &a.Name, &a.DisplayName, &a.Description,
		&a.Color, &a.IsDefault, &a.CTag, &a.SyncToken, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return domain.AddressBook{}, err
	}
	a.Tenant = t
	return a, nil
}

// NewAddressBook son los datos de creacion de una libreta de direcciones.
type NewAddressBook struct {
	OwnerSub    string
	Name        string
	DisplayName string
	Description string
	Color       string
	IsDefault   bool
}

// CreateAddressBook inserta una libreta para un dueno.
func (s *Store) CreateAddressBook(ctx context.Context, na NewAddressBook) (domain.AddressBook, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidName(na.Name) {
		return domain.AddressBook{}, domain.ErrInvalidName
	}
	if na.Color == "" {
		na.Color = "#3B82F6"
	}
	var a domain.AddressBook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`INSERT INTO address_books (tenant, owner_sub, name, display_name, description, color, is_default)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING `+abookCols,
			t.Slug, na.OwnerSub, strings.TrimSpace(na.Name), na.DisplayName, na.Description,
			na.Color, na.IsDefault)
		var e error
		a, e = scanAddressBook(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	return a, err
}

// EnsureDefaultAddressBook devuelve la libreta por defecto del dueno, creando
// una ("contacts") si aun no tiene ninguna. Idempotente.
func (s *Store) EnsureDefaultAddressBook(ctx context.Context, ownerSub string) (domain.AddressBook, error) {
	t := tenancy.MustFrom(ctx)
	var a domain.AddressBook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+abookCols+` FROM address_books
			 WHERE tenant=$1 AND owner_sub=$2 AND is_default AND deleted_at IS NULL`,
			t.Slug, ownerSub)
		got, err := scanAddressBook(row, t.Slug)
		if err == nil {
			a = got
			return nil
		}
		if !isNoRows(err) {
			return fmt.Errorf("store: leer libreta por defecto: %w", err)
		}
		insert := tx.QueryRow(ctx,
			`INSERT INTO address_books (tenant, owner_sub, name, display_name, is_default)
			 VALUES ($1, $2, 'contacts', 'Contactos', true)
			 RETURNING `+abookCols,
			t.Slug, ownerSub)
		a, err = scanAddressBook(insert, t.Slug)
		if err != nil {
			return fmt.Errorf("store: crear libreta por defecto: %w", err)
		}
		return nil
	})
	return a, err
}

// ListAddressBooks lista las libretas vivas de un dueno.
func (s *Store) ListAddressBooks(ctx context.Context, ownerSub string) ([]domain.AddressBook, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.AddressBook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+abookCols+` FROM address_books
			 WHERE tenant=$1 AND owner_sub=$2 AND deleted_at IS NULL
			 ORDER BY is_default DESC, lower(display_name), lower(name)`,
			t.Slug, ownerSub)
		if err != nil {
			return fmt.Errorf("store: listar libretas: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAddressBook(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// GetAddressBook devuelve una libreta viva del dueno por ID. Ajena =>
// ErrNotFound (anti-enumeracion).
func (s *Store) GetAddressBook(ctx context.Context, ownerSub string, id uuid.UUID) (domain.AddressBook, error) {
	t := tenancy.MustFrom(ctx)
	var a domain.AddressBook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+abookCols+` FROM address_books
			 WHERE tenant=$1 AND id=$2 AND owner_sub=$3 AND deleted_at IS NULL`,
			t.Slug, id, ownerSub)
		var e error
		a, e = scanAddressBook(row, t.Slug)
		return e
	})
	if isNoRows(err) {
		return domain.AddressBook{}, domain.ErrNotFound
	}
	return a, err
}

// GetAddressBookByName resuelve una libreta por (dueno, name) — ruta CardDAV.
func (s *Store) GetAddressBookByName(ctx context.Context, ownerSub, name string) (domain.AddressBook, error) {
	t := tenancy.MustFrom(ctx)
	var a domain.AddressBook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+abookCols+` FROM address_books
			 WHERE tenant=$1 AND owner_sub=$2 AND lower(name)=lower($3) AND deleted_at IS NULL`,
			t.Slug, ownerSub, name)
		var e error
		a, e = scanAddressBook(row, t.Slug)
		return e
	})
	if isNoRows(err) {
		return domain.AddressBook{}, domain.ErrNotFound
	}
	return a, err
}

// AddressBookPatch son los campos editables de una libreta (nil = no cambia).
type AddressBookPatch struct {
	DisplayName *string
	Description *string
	Color       *string
}

// UpdateAddressBook aplica un patch parcial a una libreta del dueno.
func (s *Store) UpdateAddressBook(ctx context.Context, ownerSub string, id uuid.UUID, p AddressBookPatch) (domain.AddressBook, error) {
	t := tenancy.MustFrom(ctx)
	var a domain.AddressBook
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE address_books SET
				display_name = COALESCE($3, display_name),
				description  = COALESCE($4, description),
				color        = COALESCE($5, color),
				updated_at   = now()
			 WHERE tenant=$1 AND id=$2 AND owner_sub=$6 AND deleted_at IS NULL
			 RETURNING `+abookCols,
			t.Slug, id, p.DisplayName, p.Description, p.Color, ownerSub)
		var e error
		a, e = scanAddressBook(row, t.Slug)
		return e
	})
	if isNoRows(err) {
		return domain.AddressBook{}, domain.ErrNotFound
	}
	return a, err
}

// DeleteAddressBook marca una libreta del dueno como borrada (soft). La por
// defecto no se puede borrar. Ajena => ErrNotFound.
func (s *Store) DeleteAddressBook(ctx context.Context, ownerSub string, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var isDefault bool
		if err := tx.QueryRow(ctx,
			`SELECT is_default FROM address_books
			 WHERE tenant=$1 AND id=$2 AND owner_sub=$3 AND deleted_at IS NULL`,
			t.Slug, id, ownerSub).Scan(&isDefault); err != nil {
			if isNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if isDefault {
			return domain.ErrDefaultImmutable
		}
		tag, err := tx.Exec(ctx,
			`UPDATE address_books SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND owner_sub=$3 AND deleted_at IS NULL`, t.Slug, id, ownerSub)
		if err != nil {
			return fmt.Errorf("store: borrar libreta: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// bumpAddressBook avanza ctag y sync_token de una libreta tras cualquier cambio
// de su contenido (debe llamarse dentro de la misma transaccion) para que
// sync-collection (RFC 6578) etiquete los cambios.
func bumpAddressBook(ctx context.Context, tx pgx.Tx, tenant string, bookID uuid.UUID) error {
	var token int64
	err := tx.QueryRow(ctx,
		`UPDATE address_books
		 SET ctag = gen_random_uuid(), sync_token = sync_token + 1, updated_at = now()
		 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL
		 RETURNING sync_token`, tenant, bookID).Scan(&token)
	if isNoRows(err) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: avanzar ctag/sync_token: %w", err)
	}
	return nil
}
