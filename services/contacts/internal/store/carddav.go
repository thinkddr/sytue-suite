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

const cardCols = `id, address_book_id, uid, resource_name, kind, etag, size_bytes, created_at, updated_at`

func scanCardMeta(row pgx.Row, t string) (domain.CardObject, error) {
	var c domain.CardObject
	if err := row.Scan(&c.ID, &c.AddressBookID, &c.UID, &c.ResourceName, &c.Kind,
		&c.ETag, &c.SizeBytes, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return domain.CardObject{}, err
	}
	c.Tenant = t
	return c, nil
}

// ListCardObjects lista los metadatos de los recursos vivos de una libreta
// (para PROPFIND Depth:1 y addressbook-multiget/query, RFC 6352).
func (s *Store) ListCardObjects(ctx context.Context, bookID uuid.UUID) ([]domain.CardObject, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.CardObject
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+cardCols+` FROM card_objects
			 WHERE tenant=$1 AND address_book_id=$2 AND deleted_at IS NULL
			 ORDER BY resource_name`,
			t.Slug, bookID)
		if err != nil {
			return fmt.Errorf("store: listar objetos CardDAV: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanCardMeta(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// GetCardObjectByResource devuelve el objeto completo (con la vCard) por su
// nombre de recurso CardDAV. Usado por GET y por addressbook-multiget.
func (s *Store) GetCardObjectByResource(ctx context.Context, bookID uuid.UUID, resourceName string) (domain.CardObject, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.CardObject
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT id, address_book_id, uid, resource_name, kind, data, etag, size_bytes, created_at, updated_at
			 FROM card_objects
			 WHERE tenant=$1 AND address_book_id=$2 AND resource_name=$3 AND deleted_at IS NULL`,
			t.Slug, bookID, resourceName)
		return row.Scan(&c.ID, &c.AddressBookID, &c.UID, &c.ResourceName, &c.Kind,
			&c.Data, &c.ETag, &c.SizeBytes, &c.CreatedAt, &c.UpdatedAt)
	})
	if isNoRows(err) {
		return domain.CardObject{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.CardObject{}, err
	}
	c.Tenant = t.Slug
	return c, nil
}

// GetCardObjectByUID devuelve el objeto completo por su UID dentro de la libreta.
func (s *Store) GetCardObjectByUID(ctx context.Context, bookID uuid.UUID, uid string) (domain.CardObject, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.CardObject
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT id, address_book_id, uid, resource_name, kind, data, etag, size_bytes, created_at, updated_at
			 FROM card_objects
			 WHERE tenant=$1 AND address_book_id=$2 AND uid=$3 AND deleted_at IS NULL`,
			t.Slug, bookID, uid)
		return row.Scan(&c.ID, &c.AddressBookID, &c.UID, &c.ResourceName, &c.Kind,
			&c.Data, &c.ETag, &c.SizeBytes, &c.CreatedAt, &c.UpdatedAt)
	})
	if isNoRows(err) {
		return domain.CardObject{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.CardObject{}, err
	}
	c.Tenant = t.Slug
	return c, nil
}

// ResolveResourceID resuelve un (libreta, resource_name) al ID del contacto o
// grupo subyacente, para que CardDAV reutilice la logica de upsert de la API.
type ResolvedResource struct {
	ObjectID  uuid.UUID
	Kind      domain.Kind
	ContactID *uuid.UUID
	GroupID   *uuid.UUID
	ETag      uuid.UUID
	Found     bool
}

// ResolveResource localiza el objeto de tarjeta por nombre de recurso y, segun
// su KIND, el contacto o grupo asociado. Si no existe, Found=false (sin error).
func (s *Store) ResolveResource(ctx context.Context, bookID uuid.UUID, resourceName string) (ResolvedResource, error) {
	t := tenancy.MustFrom(ctx)
	var rr ResolvedResource
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var kind string
		err := tx.QueryRow(ctx,
			`SELECT id, kind, etag FROM card_objects
			 WHERE tenant=$1 AND address_book_id=$2 AND resource_name=$3 AND deleted_at IS NULL`,
			t.Slug, bookID, resourceName).Scan(&rr.ObjectID, &kind, &rr.ETag)
		if isNoRows(err) {
			return nil // Found stays false
		}
		if err != nil {
			return err
		}
		rr.Found = true
		rr.Kind = domain.Kind(kind)
		if rr.Kind == domain.KindGroup {
			var gid uuid.UUID
			if err := tx.QueryRow(ctx,
				`SELECT id FROM contact_groups WHERE tenant=$1 AND object_id=$2 AND deleted_at IS NULL`,
				t.Slug, rr.ObjectID).Scan(&gid); err == nil {
				rr.GroupID = &gid
			} else if !isNoRows(err) {
				return err
			}
			return nil
		}
		var cid uuid.UUID
		if err := tx.QueryRow(ctx,
			`SELECT id FROM contacts WHERE tenant=$1 AND object_id=$2 AND deleted_at IS NULL`,
			t.Slug, rr.ObjectID).Scan(&cid); err == nil {
			rr.ContactID = &cid
		} else if !isNoRows(err) {
			return err
		}
		return nil
	})
	return rr, err
}

// SyncChange es un cambio reportado por sync-collection (RFC 6578): un recurso
// creado/modificado (Deleted=false, con su ETag actual) o borrado (Deleted=true).
type SyncChange struct {
	ResourceName string
	ETag         uuid.UUID
	Deleted      bool
}

// SyncResult es la respuesta de una consulta sync-collection.
type SyncResult struct {
	Changes   []SyncChange
	SyncToken int64
}

// SyncCollection devuelve los cambios de una libreta desde un sync_token dado
// (RFC 6578). Con sinceToken<=0 devuelve el listado completo (sync inicial). El
// soporte de borrados requiere conservar tumbas; aqui derivamos los borrados de
// los objetos marcados deleted con updated_at posterior implicito por el token
// (modelo simplificado: incluimos los vivos siempre, y los borrados desde el
// corte temporal del token solo si sinceToken>0).
func (s *Store) SyncCollection(ctx context.Context, bookID uuid.UUID, sinceToken int64) (SyncResult, error) {
	t := tenancy.MustFrom(ctx)
	var res SyncResult
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT sync_token FROM address_books WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, bookID).Scan(&res.SyncToken); err != nil {
			if isNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		// Recursos vivos (creados/modificados).
		rows, err := tx.Query(ctx,
			`SELECT resource_name, etag FROM card_objects
			 WHERE tenant=$1 AND address_book_id=$2 AND deleted_at IS NULL
			 ORDER BY resource_name`,
			t.Slug, bookID)
		if err != nil {
			return fmt.Errorf("store: sync vivos: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var ch SyncChange
			if err := rows.Scan(&ch.ResourceName, &ch.ETag); err != nil {
				return err
			}
			res.Changes = append(res.Changes, ch)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		// Borrados: en sync incremental reportamos las tumbas recientes.
		if sinceToken > 0 {
			drows, err := tx.Query(ctx,
				`SELECT resource_name, etag FROM card_objects
				 WHERE tenant=$1 AND address_book_id=$2 AND deleted_at IS NOT NULL
				 ORDER BY resource_name`,
				t.Slug, bookID)
			if err != nil {
				return fmt.Errorf("store: sync borrados: %w", err)
			}
			defer drows.Close()
			for drows.Next() {
				var ch SyncChange
				if err := drows.Scan(&ch.ResourceName, &ch.ETag); err != nil {
					return err
				}
				ch.Deleted = true
				res.Changes = append(res.Changes, ch)
			}
			if err := drows.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	return res, err
}

// DeleteByResource borra (soft) el recurso CardDAV identificado por su nombre,
// resolviendo si es contacto o grupo y validando opcionalmente el ETag
// (If-Match). Owner-scoped: delega en DeleteGroup/DeleteContact con el subject.
func (s *Store) DeleteByResource(ctx context.Context, subject string, bookID uuid.UUID, resourceName string, ifMatch *uuid.UUID) error {
	rr, err := s.ResolveResource(ctx, bookID, resourceName)
	if err != nil {
		return err
	}
	if !rr.Found {
		return domain.ErrNotFound
	}
	if ifMatch != nil && *ifMatch != rr.ETag {
		return domain.ErrPreconditionFailed
	}
	switch {
	case rr.GroupID != nil:
		return s.DeleteGroup(ctx, subject, *rr.GroupID)
	case rr.ContactID != nil:
		return s.DeleteContact(ctx, subject, *rr.ContactID)
	default:
		return domain.ErrNotFound
	}
}
