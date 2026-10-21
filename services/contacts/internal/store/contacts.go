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

const contactCols = `c.id, c.address_book_id, c.object_id, c.uid, c.formatted_name,
	c.family_name, c.given_name, c.additional_name, c.honorific_prefix, c.honorific_suffix,
	c.organization, c.title, c.nickname, c.note, c.birthday, c.photo_url,
	o.has_photo, o.etag, c.created_at, c.updated_at`

func scanContact(row pgx.Row, t string) (domain.Contact, error) {
	var c domain.Contact
	if err := row.Scan(&c.ID, &c.AddressBookID, &c.ObjectID, &c.UID, &c.FormattedName,
		&c.FamilyName, &c.GivenName, &c.AdditionalName, &c.HonorificPrefix, &c.HonorificSuffix,
		&c.Organization, &c.Title, &c.Nickname, &c.Note, &c.Birthday, &c.PhotoURL,
		&c.HasPhoto, &c.ETag, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return domain.Contact{}, err
	}
	c.Tenant = t
	return c, nil
}

// UpsertContact son los datos para crear o reemplazar un contacto y su tarjeta.
type UpsertContact struct {
	AddressBookID uuid.UUID
	// Contact es la proyeccion normalizada (incluye UID, FN, emails, telefonos).
	Contact domain.Contact
	// CardData es la vCard 4.0 canonica serializada que se guarda verbatim.
	CardData string
	// ResourceName es el nombre del recurso CardDAV (<uid>.vcf). Si vacio, se
	// deriva del UID.
	ResourceName string
}

// CreateContact inserta un contacto: crea el objeto de tarjeta (card_objects), su
// proyeccion (contacts) y sus correos/telefonos, y avanza el ctag de la libreta.
// Owner-scoped: la libreta debe pertenecer al subject (ErrNotFound si es ajena).
// Devuelve el contacto persistido con su ETag.
func (s *Store) CreateContact(ctx context.Context, subject string, in UpsertContact) (domain.Contact, error) {
	t := tenancy.MustFrom(ctx)
	if strings.TrimSpace(in.Contact.FormattedName) == "" {
		return domain.Contact{}, domain.ErrInvalidContact
	}
	if in.ResourceName == "" {
		in.ResourceName = domain.ResourceName(in.Contact.UID)
	}
	var out domain.Contact
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := assertOwnedBook(ctx, tx, t.Slug, subject, in.AddressBookID); err != nil {
			return err
		}
		var objectID uuid.UUID
		err := tx.QueryRow(ctx,
			`INSERT INTO card_objects (tenant, address_book_id, uid, resource_name, kind, data, size_bytes)
			 VALUES ($1, $2, $3, $4, 'individual', $5, $6)
			 RETURNING id`,
			t.Slug, in.AddressBookID, in.Contact.UID, in.ResourceName,
			in.CardData, len(in.CardData)).Scan(&objectID)
		if uniqueViolation(err) {
			return domain.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("store: insertar objeto de tarjeta: %w", err)
		}
		c, err := insertContactRow(ctx, tx, t.Slug, in.AddressBookID, objectID, in.Contact)
		if err != nil {
			return err
		}
		if err := replaceChannels(ctx, tx, t.Slug, c.ID, in.Contact.Emails, in.Contact.Phones); err != nil {
			return err
		}
		if err := bumpAddressBook(ctx, tx, t.Slug, in.AddressBookID); err != nil {
			return err
		}
		out, err = loadContact(ctx, tx, t.Slug, subject, c.ID)
		return err
	})
	return out, err
}

// UpdateContact reemplaza la tarjeta y la proyeccion de un contacto existente,
// validando opcionalmente el ETag previo (operacion condicional If-Match). Si
// ifMatch es no nil y no coincide, devuelve ErrPreconditionFailed. Owner-scoped
// (JOIN address_books por owner_sub, patron trash.go): un contacto ajeno del
// mismo tenant es ErrNotFound (anti-enumeracion).
func (s *Store) UpdateContact(ctx context.Context, subject string, id uuid.UUID, in UpsertContact, ifMatch *uuid.UUID) (domain.Contact, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.Contact
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var objectID, abookID, curETag uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT c.object_id, c.address_book_id, o.etag
			 FROM contacts c JOIN card_objects o ON o.id = c.object_id AND o.tenant = c.tenant
			 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
			 WHERE c.tenant=$1 AND c.id=$2 AND ab.owner_sub=$3 AND c.deleted_at IS NULL AND o.deleted_at IS NULL
			 FOR UPDATE OF c`,
			t.Slug, id, subject).Scan(&objectID, &abookID, &curETag)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: leer contacto a actualizar: %w", err)
		}
		if ifMatch != nil && *ifMatch != curETag {
			return domain.ErrPreconditionFailed
		}
		if _, err := tx.Exec(ctx,
			`UPDATE card_objects SET data=$3, size_bytes=$4, etag=gen_random_uuid(), updated_at=now()
			 WHERE tenant=$1 AND id=$2`,
			t.Slug, objectID, in.CardData, len(in.CardData)); err != nil {
			return fmt.Errorf("store: actualizar objeto de tarjeta: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE contacts SET
				uid=$3, formatted_name=$4, family_name=$5, given_name=$6, additional_name=$7,
				honorific_prefix=$8, honorific_suffix=$9, organization=$10, title=$11,
				nickname=$12, note=$13, birthday=$14, photo_url=$15, updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id, in.Contact.UID, in.Contact.FormattedName, in.Contact.FamilyName,
			in.Contact.GivenName, in.Contact.AdditionalName, in.Contact.HonorificPrefix,
			in.Contact.HonorificSuffix, in.Contact.Organization, in.Contact.Title,
			in.Contact.Nickname, in.Contact.Note, in.Contact.Birthday, in.Contact.PhotoURL); err != nil {
			return fmt.Errorf("store: actualizar contacto: %w", err)
		}
		if err := replaceChannels(ctx, tx, t.Slug, id, in.Contact.Emails, in.Contact.Phones); err != nil {
			return err
		}
		if err := bumpAddressBook(ctx, tx, t.Slug, abookID); err != nil {
			return err
		}
		out, err = loadContact(ctx, tx, t.Slug, subject, id)
		return err
	})
	return out, err
}

// ListContacts lista los contactos vivos de una libreta del subject en orden
// estable (family/given/FN/id). limit acota el tamano de pagina (1..500, por
// defecto 100). Los correos y telefonos se adjuntan en lote (sin N+1).
func (s *Store) ListContacts(ctx context.Context, subject string, bookID uuid.UUID, limit int) ([]domain.Contact, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []domain.Contact
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := assertOwnedBook(ctx, tx, t.Slug, subject, bookID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT `+contactCols+`
			 FROM contacts c JOIN card_objects o ON o.id = c.object_id AND o.tenant = c.tenant
			 WHERE c.tenant=$1 AND c.address_book_id=$2 AND c.deleted_at IS NULL
			 ORDER BY lower(c.family_name), lower(c.given_name), lower(c.formatted_name), c.id
			 LIMIT $3`,
			t.Slug, bookID, limit)
		if err != nil {
			return fmt.Errorf("store: listar contactos: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanContact(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return attachChannels(ctx, tx, t.Slug, out)
	})
	return out, err
}

// ListOwnerContactsPage lista una PAGINA de los contactos vivos del subject en
// TODAS sus libretas (con correos y telefonos), ordenados de forma estable para
// que la paginacion sea determinista. Owner-scoped por el JOIN con address_books:
// nunca devuelve contactos de otro usuario del tenant, ni siquiera del mismo.
//
// Es lo que consume la capa de compatibilidad (People API / Graph contacts), donde
// «mis contactos» no se acota a una libreta. limit se acota a 1..500; offset<0
// cuenta como 0.
//
// ponytail: paginacion por OFFSET (el orden es por nombre, sin clave keyset
// natural); si un usuario acumula decenas de miles de contactos y las paginas
// profundas se notan, el salto es a keyset por (lower(family_name), id).
func (s *Store) ListOwnerContactsPage(ctx context.Context, subject string, limit, offset int) ([]domain.Contact, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var out []domain.Contact
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+contactCols+`
			 FROM contacts c JOIN card_objects o ON o.id = c.object_id AND o.tenant = c.tenant
			 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
			 WHERE c.tenant=$1 AND ab.owner_sub=$2 AND c.deleted_at IS NULL AND ab.deleted_at IS NULL
			 ORDER BY lower(c.family_name), lower(c.given_name), lower(c.formatted_name), c.id
			 LIMIT $3 OFFSET $4`,
			t.Slug, subject, limit, offset)
		if err != nil {
			return fmt.Errorf("store: listar contactos del dueno: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanContact(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return attachChannels(ctx, tx, t.Slug, out)
	})
	return out, err
}

// GetContact devuelve un contacto vivo del subject por ID (con correos y
// telefonos). Ajeno => ErrNotFound.
func (s *Store) GetContact(ctx context.Context, subject string, id uuid.UUID) (domain.Contact, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.Contact
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		c, e = loadContact(ctx, tx, t.Slug, subject, id)
		return e
	})
	return c, err
}

// DeleteContact marca un contacto del subject y su objeto de tarjeta como
// borrados (soft) y avanza el ctag de la libreta. Ajeno => ErrNotFound.
func (s *Store) DeleteContact(ctx context.Context, subject string, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var objectID, abookID uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT c.object_id, c.address_book_id FROM contacts c
			 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
			 WHERE c.tenant=$1 AND c.id=$2 AND ab.owner_sub=$3 AND c.deleted_at IS NULL`,
			t.Slug, id, subject).Scan(&objectID, &abookID)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE contacts SET deleted_at=now(), updated_at=now() WHERE tenant=$1 AND id=$2`,
			t.Slug, id); err != nil {
			return fmt.Errorf("store: borrar contacto: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE card_objects SET deleted_at=now(), updated_at=now() WHERE tenant=$1 AND id=$2`,
			t.Slug, objectID); err != nil {
			return fmt.Errorf("store: borrar objeto de tarjeta: %w", err)
		}
		err = bumpAddressBook(ctx, tx, t.Slug, abookID)
		return err
	})
}

// SearchContacts busca SOLO en las libretas del subject (JOIN address_books por
// owner_sub, patron trash.go: contacts no tiene modelo de comparticion, asi que
// los contactos de otros usuarios del tenant nunca se sirven), opcionalmente
// acotado a una libreta. Combina full-text (tsvector `search`,
// palabras/prefijos) con trigramas (pg_trgm sobre `search_text`, ILIKE '%q%'):
// así casa infijos ("ana" -> "Mariana") y tolera erratas, ambos servidos por
// índice GIN (sin seq-scan). Orden: coincidencia por subcadena primero, luego
// similaridad trigram, luego rango FTS, luego nombre. limit acota el resultado
// (1..200, por defecto 50).
func (s *Store) SearchContacts(ctx context.Context, subject, query string, bookID *uuid.UUID, limit int) ([]domain.Contact, error) {
	t := tenancy.MustFrom(ctx)
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []domain.Contact
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// ponytail: ILIKE sin escapar %/_ (metacaracteres LIKE), igual que
		// mail/docs/chat; es relevancia de búsqueda, no inyección (parametrizado).
		rows, err := tx.Query(ctx,
			`SELECT `+contactCols+`
			 FROM contacts c JOIN card_objects o ON o.id = c.object_id AND o.tenant = c.tenant
			 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
			 WHERE c.tenant=$1 AND ab.owner_sub=$2 AND c.deleted_at IS NULL
			   AND ($3::uuid IS NULL OR c.address_book_id = $3)
			   AND (c.search @@ websearch_to_tsquery('simple', plegar($4))
			        OR c.search_text LIKE '%' || plegar($4) || '%')
			 ORDER BY (c.search_text LIKE '%' || plegar($4) || '%') DESC,
			          similarity(c.search_text, plegar($4)) DESC,
			          ts_rank(c.search, websearch_to_tsquery('simple', plegar($4))) DESC,
			          lower(c.formatted_name)
			 LIMIT $5`,
			t.Slug, subject, bookID, query, limit)
		if err != nil {
			return fmt.Errorf("store: buscar contactos: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanContact(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return attachChannels(ctx, tx, t.Slug, out)
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Helpers internos (siempre dentro de una transaccion con app.tenant fijado)
// ---------------------------------------------------------------------------

// assertOwnedBook verifica que la libreta exista, este viva Y pertenezca al
// subject: es la verja de dueno unica de crear/listar para todos los llamantes
// (REST y CardDAV). Libreta ajena => ErrNotFound (anti-enumeracion).
func assertOwnedBook(ctx context.Context, tx pgx.Tx, tenant, subject string, bookID uuid.UUID) error {
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM address_books
		 WHERE tenant=$1 AND id=$2 AND owner_sub=$3 AND deleted_at IS NULL)`,
		tenant, bookID, subject).Scan(&exists); err != nil {
		return fmt.Errorf("store: comprobar libreta: %w", err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	return nil
}

// insertContactRow inserta la fila de proyeccion de un contacto.
func insertContactRow(ctx context.Context, tx pgx.Tx, tenant string, bookID, objectID uuid.UUID, c domain.Contact) (domain.Contact, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx,
		`INSERT INTO contacts (tenant, address_book_id, object_id, uid, formatted_name,
			family_name, given_name, additional_name, honorific_prefix, honorific_suffix,
			organization, title, nickname, note, birthday, photo_url)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 RETURNING id`,
		tenant, bookID, objectID, c.UID, c.FormattedName, c.FamilyName, c.GivenName,
		c.AdditionalName, c.HonorificPrefix, c.HonorificSuffix, c.Organization, c.Title,
		c.Nickname, c.Note, c.Birthday, c.PhotoURL).Scan(&id)
	if err != nil {
		return domain.Contact{}, fmt.Errorf("store: insertar contacto: %w", err)
	}
	c.ID = id
	c.Tenant = tenant
	c.AddressBookID = bookID
	c.ObjectID = objectID
	return c, nil
}

// replaceChannels reemplaza por completo los correos y telefonos de un contacto.
func replaceChannels(ctx context.Context, tx pgx.Tx, tenant string, contactID uuid.UUID, emails []domain.Email, phones []domain.Phone) error {
	if _, err := tx.Exec(ctx, `DELETE FROM contact_emails WHERE tenant=$1 AND contact_id=$2`, tenant, contactID); err != nil {
		return fmt.Errorf("store: limpiar correos: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM contact_phones WHERE tenant=$1 AND contact_id=$2`, tenant, contactID); err != nil {
		return fmt.Errorf("store: limpiar telefonos: %w", err)
	}
	for i, e := range emails {
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact_emails (tenant, contact_id, email, type_label, is_primary, position)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			tenant, contactID, e.Email, string(e.Type), e.IsPrimary, i); err != nil {
			return fmt.Errorf("store: insertar correo: %w", err)
		}
	}
	for i, p := range phones {
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact_phones (tenant, contact_id, phone, type_label, is_primary, position)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			tenant, contactID, p.Phone, string(p.Type), p.IsPrimary, i); err != nil {
			return fmt.Errorf("store: insertar telefono: %w", err)
		}
	}
	return nil
}

// loadContact carga un contacto vivo del subject por ID con sus correos y
// telefonos (owner-scoped: JOIN address_books por owner_sub).
func loadContact(ctx context.Context, tx pgx.Tx, tenant, subject string, id uuid.UUID) (domain.Contact, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+contactCols+`
		 FROM contacts c JOIN card_objects o ON o.id = c.object_id AND o.tenant = c.tenant
		 JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
		 WHERE c.tenant=$1 AND c.id=$2 AND ab.owner_sub=$3 AND c.deleted_at IS NULL`,
		tenant, id, subject)
	c, err := scanContact(row, tenant)
	if isNoRows(err) {
		return domain.Contact{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Contact{}, err
	}
	batch := []domain.Contact{c}
	if err := attachChannels(ctx, tx, tenant, batch); err != nil {
		return domain.Contact{}, err
	}
	return batch[0], nil
}

// attachChannels rellena correos/telefonos de un lote de contactos en dos
// consultas (sin N+1). Muta los elementos del slice in situ.
func attachChannels(ctx context.Context, tx pgx.Tx, tenant string, contacts []domain.Contact) error {
	if len(contacts) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(contacts))
	idx := make(map[uuid.UUID]int, len(contacts))
	for i := range contacts {
		ids[i] = contacts[i].ID
		idx[contacts[i].ID] = i
		contacts[i].Emails = nil
		contacts[i].Phones = nil
	}

	erows, err := tx.Query(ctx,
		`SELECT contact_id, email, type_label, is_primary
		 FROM contact_emails WHERE tenant=$1 AND contact_id = ANY($2)
		 ORDER BY position`,
		tenant, ids)
	if err != nil {
		return fmt.Errorf("store: leer correos: %w", err)
	}
	for erows.Next() {
		var cid uuid.UUID
		var e domain.Email
		if err := erows.Scan(&cid, &e.Email, &e.Type, &e.IsPrimary); err != nil {
			erows.Close()
			return err
		}
		if i, ok := idx[cid]; ok {
			contacts[i].Emails = append(contacts[i].Emails, e)
		}
	}
	erows.Close()
	if err := erows.Err(); err != nil {
		return err
	}

	prows, err := tx.Query(ctx,
		`SELECT contact_id, phone, type_label, is_primary
		 FROM contact_phones WHERE tenant=$1 AND contact_id = ANY($2)
		 ORDER BY position`,
		tenant, ids)
	if err != nil {
		return fmt.Errorf("store: leer telefonos: %w", err)
	}
	defer prows.Close()
	for prows.Next() {
		var cid uuid.UUID
		var p domain.Phone
		if err := prows.Scan(&cid, &p.Phone, &p.Type, &p.IsPrimary); err != nil {
			return err
		}
		if i, ok := idx[cid]; ok {
			contacts[i].Phones = append(contacts[i].Phones, p)
		}
	}
	return prows.Err()
}
