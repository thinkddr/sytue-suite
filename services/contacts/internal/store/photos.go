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

// Foto de contacto: se guarda EMBEBIDA en la PHOTO de la vCard (card_objects.data)
// como data URI base64 — sin object storage. Todas las operaciones direccionan por
// UID (lo que maneja el frontend via CardDAV) y SOLO sobre contactos del dueno
// (JOIN a address_books por owner_sub): un usuario no toca la foto de otro. Editar
// la foto avanza el etag y el ctag/sync_token (la tarjeta cambia -> CardDAV lo ve).

// loadOwnedCard trae el objeto de tarjeta (id, libreta, etag, data) del contacto
// vivo con ese UID propiedad del subject. ErrNotFound si no existe / es ajeno.
// forUpdate bloquea la fila (FOR UPDATE) en los caminos de escritura; en la
// lectura (servir el avatar) se omite para no serializar peticiones concurrentes.
func loadOwnedCard(ctx context.Context, tx pgx.Tx, tenant, subject, uid string, forUpdate bool) (objID, bookID, etag uuid.UUID, data string, err error) {
	uid = domain.CleanUID(strings.TrimSpace(uid))
	if uid == "" {
		return uuid.Nil, uuid.Nil, uuid.Nil, "", domain.ErrNotFound
	}
	q := `SELECT o.id, o.address_book_id, o.etag, o.data
		 FROM contacts c
		 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
		 JOIN card_objects o ON o.id = c.object_id AND o.tenant = c.tenant
		 WHERE c.tenant=$1 AND ab.owner_sub=$2 AND c.uid=$3 AND c.deleted_at IS NULL AND o.deleted_at IS NULL`
	if forUpdate {
		q += ` FOR UPDATE OF o`
	}
	err = tx.QueryRow(ctx, q, tenant, subject, uid).Scan(&objID, &bookID, &etag, &data)
	if isNoRows(err) {
		return uuid.Nil, uuid.Nil, uuid.Nil, "", domain.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, "", fmt.Errorf("store: leer tarjeta del contacto: %w", err)
	}
	return objID, bookID, etag, data, nil
}

// SetContactPhoto incrusta el data URI dado como PHOTO de la vCard del contacto
// (owner-scoped por UID) y persiste la tarjeta. El data URI DEBE venir ya validado
// (tipo raster + tamano) por la capa HTTP; aqui se reincrusta tal cual.
func (s *Store) SetContactPhoto(ctx context.Context, subject, uid, dataURI string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		objID, bookID, _, data, err := loadOwnedCard(ctx, tx, t.Slug, subject, uid, true)
		if err != nil {
			return err
		}
		card, err := domain.SetCardPhoto(data, dataURI)
		if err != nil {
			return err
		}
		return persistCard(ctx, tx, t.Slug, objID, bookID, card)
	})
}

// RemoveContactPhoto elimina la PHOTO de la vCard del contacto (owner-scoped por
// UID) y persiste la tarjeta. Idempotente.
func (s *Store) RemoveContactPhoto(ctx context.Context, subject, uid string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		objID, bookID, _, data, err := loadOwnedCard(ctx, tx, t.Slug, subject, uid, true)
		if err != nil {
			return err
		}
		card, err := domain.RemoveCardPhoto(data)
		if err != nil {
			return err
		}
		return persistCard(ctx, tx, t.Slug, objID, bookID, card)
	})
}

// GetContactPhoto devuelve el data URI de la PHOTO embebida del contacto
// (owner-scoped por UID) y el etag de su tarjeta (para cache condicional). Si el
// contacto no tiene foto embebida, ErrNotFound.
func (s *Store) GetContactPhoto(ctx context.Context, subject, uid string) (dataURI string, etag uuid.UUID, err error) {
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, _, et, data, e := loadOwnedCard(ctx, tx, t.Slug, subject, uid, false)
		if e != nil {
			return e
		}
		if !domain.CardHasEmbeddedPhoto(data) {
			return domain.ErrNotFound
		}
		du, e := domain.CardPhotoDataURI(data)
		if e != nil {
			return e
		}
		dataURI, etag = du, et
		return nil
	})
	return dataURI, etag, err
}

// persistCard guarda una tarjeta re-serializada (nueva data), avanza su etag y el
// ctag/sync_token de la libreta. Debe llamarse dentro de la transaccion.
func persistCard(ctx context.Context, tx pgx.Tx, tenant string, objectID, bookID uuid.UUID, card string) error {
	if _, err := tx.Exec(ctx,
		`UPDATE card_objects SET data=$3, size_bytes=$4, etag=gen_random_uuid(), updated_at=now()
		 WHERE tenant=$1 AND id=$2`,
		tenant, objectID, card, len(card)); err != nil {
		return fmt.Errorf("store: persistir tarjeta: %w", err)
	}
	return bumpAddressBook(ctx, tx, tenant, bookID)
}
