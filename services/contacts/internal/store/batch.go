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

// Operaciones EN LOTE (multiseleccion). Cada accion es UNA sola consulta con
// `uid = ANY($uids)` (sin N+1) y SIEMPRE acotada al dueno (subject): el JOIN a
// address_books por owner_sub descarta los UID ajenos en silencio, asi un usuario
// nunca opera sobre contactos de otro aunque compartan tenant. El frontend maneja
// UID (no el id de proyeccion), como ya hacen favoritos y la pertenencia a grupos.

// cleanUIDs normaliza y deduplica una lista de UID (quita el prefijo urn:uuid: y
// los vacios). El frontend lee el UID de la vCard, que puede llegar prefijado.
func cleanUIDs(uids []string) []string {
	seen := make(map[string]struct{}, len(uids))
	out := make([]string, 0, len(uids))
	for _, u := range uids {
		u = domain.CleanUID(strings.TrimSpace(u))
		if u == "" {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

// TrashContacts envia a la papelera (borrado suave) TODOS los contactos vivos del
// dueno cuyos UID esten en la lista, en UNA operacion: marca deleted_at en el
// contacto y su tarjeta y avanza el ctag/sync_token de cada libreta afectada.
// Reusa el mismo soft-delete que DeleteContact (la papelera los lista y se pueden
// restaurar). Devuelve cuantos se enviaron a la papelera (los UID ajenos o
// inexistentes se omiten). CTEs data-modifying: `targets` se evalua una vez sobre
// el estado previo y las tres escrituras la referencian.
func (s *Store) TrashContacts(ctx context.Context, subject string, uids []string) (int, error) {
	t := tenancy.MustFrom(ctx)
	uids = cleanUIDs(uids)
	if len(uids) == 0 {
		return 0, nil
	}
	var n int
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`WITH targets AS (
			   SELECT c.id, c.object_id, c.address_book_id
			   FROM contacts c
			   JOIN address_books ab ON ab.id = c.address_book_id AND ab.tenant = c.tenant
			   WHERE c.tenant=$1 AND ab.owner_sub=$2 AND c.uid = ANY($3) AND c.deleted_at IS NULL
			 ),
			 del_c AS (
			   UPDATE contacts SET deleted_at=now(), updated_at=now()
			   WHERE tenant=$1 AND id IN (SELECT id FROM targets)
			   RETURNING id
			 ),
			 del_o AS (
			   UPDATE card_objects SET deleted_at=now(), updated_at=now()
			   WHERE tenant=$1 AND id IN (SELECT object_id FROM targets)
			   RETURNING id
			 ),
			 bump AS (
			   UPDATE address_books
			   SET ctag=gen_random_uuid(), sync_token=sync_token+1, updated_at=now()
			   WHERE tenant=$1 AND id IN (SELECT DISTINCT address_book_id FROM targets)
			   RETURNING id
			 )
			 SELECT count(*) FROM del_c`,
			t.Slug, subject, uids).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("store: papelera en lote: %w", err)
	}
	return n, nil
}

// SetFavorites destaca (on=true) o quita el destacado (on=false) de TODOS los
// contactos vivos del dueno cuyos UID esten en la lista, en UNA sola UPDATE con
// `uid = ANY`. Owner-scoped (subquery a address_books por owner_sub): los UID
// ajenos se ignoran. Devuelve cuantos cambiaron. is_favorite no forma parte de la
// vCard/CardDAV, asi que NO avanza el ctag (igual que SetFavorite).
func (s *Store) SetFavorites(ctx context.Context, subject string, uids []string, on bool) (int, error) {
	t := tenancy.MustFrom(ctx)
	uids = cleanUIDs(uids)
	if len(uids) == 0 {
		return 0, nil
	}
	var n int64
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE contacts SET is_favorite=$4, updated_at=now()
			 WHERE tenant=$1 AND uid = ANY($3) AND deleted_at IS NULL
			   AND address_book_id IN (
			       SELECT id FROM address_books
			       WHERE tenant=$1 AND owner_sub=$2 AND deleted_at IS NULL)`,
			t.Slug, subject, uids, on)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("store: favoritos en lote: %w", err)
	}
	return int(n), nil
}

// ModifyGroupMembers anade (add=true) o quita (add=false) EN LOTE los contactos
// con esos UID a un grupo del dueno, y mantiene sincronizada la vCard del grupo
// (propiedad MEMBER, RFC 6350 6.6.5). Owner-scoped: el grupo debe pertenecer a una
// libreta del subject; si no, ErrNotFound. La pertenencia se toca en UNA consulta
// (insert masivo con unnest / delete con `contact_uid = ANY`); luego se re-serializa
// la tarjeta del grupo con la lista completa de miembros y se avanza el ctag.
func (s *Store) ModifyGroupMembers(ctx context.Context, subject string, groupID uuid.UUID, uids []string, add bool) (domain.Group, error) {
	t := tenancy.MustFrom(ctx)
	uids = cleanUIDs(uids)
	var out domain.Group
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Bloquea el grupo del dueno (JOIN por owner_sub) y trae lo necesario para
		// re-serializar su tarjeta.
		var objectID, abookID uuid.UUID
		var name, gUID, oldData string
		err := tx.QueryRow(ctx,
			`SELECT g.object_id, g.address_book_id, g.uid, g.name, o.data
			 FROM contact_groups g
			 JOIN address_books ab ON ab.id = g.address_book_id AND ab.tenant = g.tenant
			 JOIN card_objects o ON o.id = g.object_id AND o.tenant = g.tenant
			 WHERE g.tenant=$1 AND g.id=$2 AND ab.owner_sub=$3 AND g.deleted_at IS NULL
			 FOR UPDATE OF g`,
			t.Slug, groupID, subject).Scan(&objectID, &abookID, &gUID, &name, &oldData)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: leer grupo a modificar: %w", err)
		}

		if len(uids) > 0 {
			if add {
				// Insert masivo: un miembro por UID, resolviendo el contact_id local
				// si existe en la libreta del grupo. ON CONFLICT DO NOTHING respeta la
				// unicidad (group_id, contact_uid) sin fallar por duplicados.
				if _, err := tx.Exec(ctx,
					`INSERT INTO contact_group_members (tenant, group_id, contact_uid, contact_id)
					 SELECT $1, $2, u,
					   (SELECT id FROM contacts WHERE tenant=$1 AND address_book_id=$4 AND uid=u AND deleted_at IS NULL LIMIT 1)
					 FROM unnest($3::text[]) AS u
					 ON CONFLICT (tenant, group_id, contact_uid) DO NOTHING`,
					t.Slug, groupID, uids, abookID); err != nil {
					return fmt.Errorf("store: anadir miembros en lote: %w", err)
				}
			} else if _, err := tx.Exec(ctx,
				`DELETE FROM contact_group_members
				 WHERE tenant=$1 AND group_id=$2 AND contact_uid = ANY($3)`,
				t.Slug, groupID, uids); err != nil {
				return fmt.Errorf("store: quitar miembros en lote: %w", err)
			}
		}

		// Lista completa de miembros tras el cambio, para re-serializar la vCard.
		members, err := groupMemberUIDs(ctx, tx, t.Slug, groupID)
		if err != nil {
			return err
		}
		card, err := domain.EncodeGroup(domain.Group{UID: gUID, Name: name, Members: members}, oldData)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE card_objects SET data=$3, size_bytes=$4, etag=gen_random_uuid(), updated_at=now()
			 WHERE tenant=$1 AND id=$2`,
			t.Slug, objectID, card, len(card)); err != nil {
			return fmt.Errorf("store: actualizar tarjeta de grupo: %w", err)
		}
		if err := bumpAddressBook(ctx, tx, t.Slug, abookID); err != nil {
			return err
		}
		out, err = loadGroup(ctx, tx, t.Slug, subject, groupID)
		return err
	})
	return out, err
}

// groupMemberUIDs devuelve los UID de los miembros de un grupo (orden estable).
func groupMemberUIDs(ctx context.Context, tx pgx.Tx, tenant string, groupID uuid.UUID) ([]string, error) {
	rows, err := tx.Query(ctx,
		`SELECT contact_uid FROM contact_group_members
		 WHERE tenant=$1 AND group_id=$2 ORDER BY created_at`,
		tenant, groupID)
	if err != nil {
		return nil, fmt.Errorf("store: leer miembros del grupo: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}
