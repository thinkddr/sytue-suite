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

// UpsertGroup son los datos para crear o reemplazar un grupo y su tarjeta
// (vCard KIND=group). Members son los UID de los contactos miembros.
type UpsertGroup struct {
	AddressBookID uuid.UUID
	Group         domain.Group
	CardData      string
	ResourceName  string
}

// CreateGroup inserta un grupo: crea su objeto de tarjeta (KIND=group), la fila
// de grupo y la pertenencia (MEMBER por UID), y avanza el ctag de la libreta.
// Owner-scoped: la libreta debe pertenecer al subject.
func (s *Store) CreateGroup(ctx context.Context, subject string, in UpsertGroup) (domain.Group, error) {
	t := tenancy.MustFrom(ctx)
	if strings.TrimSpace(in.Group.Name) == "" {
		return domain.Group{}, domain.ErrInvalidGroup
	}
	if in.ResourceName == "" {
		in.ResourceName = domain.ResourceName(in.Group.UID)
	}
	var out domain.Group
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := assertOwnedBook(ctx, tx, t.Slug, subject, in.AddressBookID); err != nil {
			return err
		}
		var objectID uuid.UUID
		err := tx.QueryRow(ctx,
			`INSERT INTO card_objects (tenant, address_book_id, uid, resource_name, kind, data, size_bytes)
			 VALUES ($1, $2, $3, $4, 'group', $5, $6)
			 RETURNING id`,
			t.Slug, in.AddressBookID, in.Group.UID, in.ResourceName,
			in.CardData, len(in.CardData)).Scan(&objectID)
		if uniqueViolation(err) {
			return domain.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("store: insertar objeto de grupo: %w", err)
		}
		var groupID uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO contact_groups (tenant, address_book_id, object_id, uid, name)
			 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			t.Slug, in.AddressBookID, objectID, in.Group.UID, in.Group.Name).Scan(&groupID); err != nil {
			return fmt.Errorf("store: insertar grupo: %w", err)
		}
		if err := replaceMembers(ctx, tx, t.Slug, groupID, in.AddressBookID, in.Group.Members); err != nil {
			return err
		}
		if err := bumpAddressBook(ctx, tx, t.Slug, in.AddressBookID); err != nil {
			return err
		}
		out, err = loadGroup(ctx, tx, t.Slug, subject, groupID)
		return err
	})
	return out, err
}

// UpdateGroup reemplaza la tarjeta, el nombre y la pertenencia de un grupo del
// subject, validando opcionalmente el ETag previo (If-Match). Ajeno =>
// ErrNotFound (JOIN address_books por owner_sub, patron ModifyGroupMembers).
func (s *Store) UpdateGroup(ctx context.Context, subject string, id uuid.UUID, in UpsertGroup, ifMatch *uuid.UUID) (domain.Group, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.Group
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var objectID, abookID, curETag uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT g.object_id, g.address_book_id, o.etag
			 FROM contact_groups g JOIN card_objects o ON o.id = g.object_id AND o.tenant = g.tenant
			 JOIN address_books ab ON ab.id = g.address_book_id AND ab.tenant = g.tenant
			 WHERE g.tenant=$1 AND g.id=$2 AND ab.owner_sub=$3 AND g.deleted_at IS NULL AND o.deleted_at IS NULL
			 FOR UPDATE OF g`,
			t.Slug, id, subject).Scan(&objectID, &abookID, &curETag)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: leer grupo a actualizar: %w", err)
		}
		if ifMatch != nil && *ifMatch != curETag {
			return domain.ErrPreconditionFailed
		}
		if _, err := tx.Exec(ctx,
			`UPDATE card_objects SET data=$3, size_bytes=$4, etag=gen_random_uuid(), updated_at=now()
			 WHERE tenant=$1 AND id=$2`,
			t.Slug, objectID, in.CardData, len(in.CardData)); err != nil {
			return fmt.Errorf("store: actualizar objeto de grupo: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE contact_groups SET name=$3, updated_at=now() WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id, in.Group.Name); err != nil {
			return fmt.Errorf("store: actualizar grupo: %w", err)
		}
		if err := replaceMembers(ctx, tx, t.Slug, id, abookID, in.Group.Members); err != nil {
			return err
		}
		if err := bumpAddressBook(ctx, tx, t.Slug, abookID); err != nil {
			return err
		}
		out, err = loadGroup(ctx, tx, t.Slug, subject, id)
		return err
	})
	return out, err
}

// ListGroups lista los grupos vivos de una libreta del subject (con miembros).
func (s *Store) ListGroups(ctx context.Context, subject string, bookID uuid.UUID) ([]domain.Group, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Group
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := assertOwnedBook(ctx, tx, t.Slug, subject, bookID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT g.id, g.address_book_id, g.object_id, g.uid, g.name, o.etag, g.created_at, g.updated_at
			 FROM contact_groups g JOIN card_objects o ON o.id = g.object_id AND o.tenant = g.tenant
			 WHERE g.tenant=$1 AND g.address_book_id=$2 AND g.deleted_at IS NULL
			 ORDER BY lower(g.name)`,
			t.Slug, bookID)
		if err != nil {
			return fmt.Errorf("store: listar grupos: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			g, err := scanGroup(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, g)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return attachMembers(ctx, tx, t.Slug, out)
	})
	return out, err
}

// GetGroup devuelve un grupo vivo del subject por ID (con miembros).
func (s *Store) GetGroup(ctx context.Context, subject string, id uuid.UUID) (domain.Group, error) {
	t := tenancy.MustFrom(ctx)
	var g domain.Group
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		g, e = loadGroup(ctx, tx, t.Slug, subject, id)
		return e
	})
	return g, err
}

// DeleteGroup marca un grupo del subject y su objeto de tarjeta como borrados
// (soft). Ajeno => ErrNotFound.
func (s *Store) DeleteGroup(ctx context.Context, subject string, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var objectID, abookID uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT g.object_id, g.address_book_id FROM contact_groups g
			 JOIN address_books ab ON ab.id = g.address_book_id AND ab.tenant = g.tenant
			 WHERE g.tenant=$1 AND g.id=$2 AND ab.owner_sub=$3 AND g.deleted_at IS NULL`,
			t.Slug, id, subject).Scan(&objectID, &abookID)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE contact_groups SET deleted_at=now(), updated_at=now() WHERE tenant=$1 AND id=$2`,
			t.Slug, id); err != nil {
			return fmt.Errorf("store: borrar grupo: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE card_objects SET deleted_at=now(), updated_at=now() WHERE tenant=$1 AND id=$2`,
			t.Slug, objectID); err != nil {
			return fmt.Errorf("store: borrar objeto de grupo: %w", err)
		}
		err = bumpAddressBook(ctx, tx, t.Slug, abookID)
		return err
	})
}

// ---------------------------------------------------------------------------
// Helpers internos
// ---------------------------------------------------------------------------

func scanGroup(row pgx.Row, t string) (domain.Group, error) {
	var g domain.Group
	if err := row.Scan(&g.ID, &g.AddressBookID, &g.ObjectID, &g.UID, &g.Name,
		&g.ETag, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return domain.Group{}, err
	}
	g.Tenant = t
	return g, nil
}

// replaceMembers reemplaza por completo la pertenencia de un grupo, resolviendo
// cada UID al contact_id local si existe (RFC 6350 6.6.5: MEMBER por UID).
func replaceMembers(ctx context.Context, tx pgx.Tx, tenant string, groupID, bookID uuid.UUID, members []string) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM contact_group_members WHERE tenant=$1 AND group_id=$2`, tenant, groupID); err != nil {
		return fmt.Errorf("store: limpiar miembros: %w", err)
	}
	// Normaliza y deduplica ANTES de tocar la base: un grupo puede traer miles de UID
	// —una lista de distribución de toda la empresa lo es— y un INSERT por miembro son
	// miles de viajes de ida y vuelta dentro de la transacción, con el grupo bloqueado
	// mientras tanto.
	seen := make(map[string]struct{}, len(members))
	uids := make([]string, 0, len(members))
	for _, uid := range members {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		if _, dup := seen[uid]; dup {
			continue
		}
		seen[uid] = struct{}{}
		uids = append(uids, uid)
	}
	if len(uids) == 0 {
		return nil
	}
	// Una sola sentencia: `unnest` despliega el array y el LEFT JOIN resuelve cada UID a
	// su contacto local en el mismo paso.
	//
	// El JOIN es EXTERNO porque un MEMBER puede apuntar a alguien que no está en esta
	// libreta (RFC 6350 §6.6.5): esa fila se guarda igual con contact_id nulo, y con un
	// JOIN interno desaparecería en silencio.
	//
	// Y lleva DISTINCT ON porque el bucle que sustituye resolvía con `LIMIT 1`: la
	// unicidad de `uid` está declarada sobre `card_objects`, NO sobre `contacts` —cuyo
	// índice por uid no es único—, así que dos contactos vivos con el mismo UID darían
	// DOS filas de pertenencia donde antes había una. Comprobado en un Postgres real
	// antes de escribir esto; no se depende de una invariante que nadie declara.
	if _, err := tx.Exec(ctx,
		`INSERT INTO contact_group_members (tenant, group_id, contact_uid, contact_id)
		 SELECT DISTINCT ON (m.uid) $1, $2, m.uid, c.id
		   FROM unnest($3::text[]) AS m(uid)
		   LEFT JOIN contacts c
		     ON c.tenant = $1 AND c.address_book_id = $4 AND c.uid = m.uid
		    AND c.deleted_at IS NULL
		  ORDER BY m.uid, c.id`,
		tenant, groupID, uids, bookID); err != nil {
		return fmt.Errorf("store: insertar miembros: %w", err)
	}
	return nil
}

// loadGroup carga un grupo vivo del subject (owner-scoped via address_books).
func loadGroup(ctx context.Context, tx pgx.Tx, tenant, subject string, id uuid.UUID) (domain.Group, error) {
	row := tx.QueryRow(ctx,
		`SELECT g.id, g.address_book_id, g.object_id, g.uid, g.name, o.etag, g.created_at, g.updated_at
		 FROM contact_groups g JOIN card_objects o ON o.id = g.object_id AND o.tenant = g.tenant
		 JOIN address_books ab ON ab.id = g.address_book_id AND ab.tenant = g.tenant
		 WHERE g.tenant=$1 AND g.id=$2 AND ab.owner_sub=$3 AND g.deleted_at IS NULL`,
		tenant, id, subject)
	g, err := scanGroup(row, tenant)
	if isNoRows(err) {
		return domain.Group{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Group{}, err
	}
	batch := []domain.Group{g}
	if err := attachMembers(ctx, tx, tenant, batch); err != nil {
		return domain.Group{}, err
	}
	return batch[0], nil
}

// attachMembers rellena los UID de miembros de un lote de grupos (sin N+1).
func attachMembers(ctx context.Context, tx pgx.Tx, tenant string, groups []domain.Group) error {
	if len(groups) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(groups))
	idx := make(map[uuid.UUID]int, len(groups))
	for i := range groups {
		ids[i] = groups[i].ID
		idx[groups[i].ID] = i
		groups[i].Members = []string{}
	}
	rows, err := tx.Query(ctx,
		`SELECT group_id, contact_uid FROM contact_group_members
		 WHERE tenant=$1 AND group_id = ANY($2) ORDER BY created_at`,
		tenant, ids)
	if err != nil {
		return fmt.Errorf("store: leer miembros: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var gid uuid.UUID
		var uid string
		if err := rows.Scan(&gid, &uid); err != nil {
			return err
		}
		if i, ok := idx[gid]; ok {
			groups[i].Members = append(groups[i].Members, uid)
		}
	}
	return rows.Err()
}
