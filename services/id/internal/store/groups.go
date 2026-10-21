// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Aislamiento por tenant (docs/CONVENTIONS.md §0.7): Sytue ID es la FUENTE de
// identidad y NO usa RLS de sesión (eso es aguas abajo, en los servicios
// consumidores). Aísla por columna org_id en CADA consulta, igual que
// users/memberships. Por eso TODOS los métodos de grupos reciben orgID (el de la
// org activa del token, resuelto en el handler vía activeOrg — NUNCA del path) y
// lo cruzan con groups.org_id: un group_id o user_id de otro tenant no liga
// (devuelve ErrNotFound / 0 filas), nunca una fuga cross-tenant.

const groupCols = `id, org_id, name, coalesce(description, ''), coalesce(external_id, ''), created_at, coalesce(membership_rule::text, '')`

func scanGroup(row pgx.Row) (domain.Group, error) {
	var g domain.Group
	var ruleJSON string
	if err := row.Scan(&g.ID, &g.OrgID, &g.Name, &g.Description, &g.ExternalID, &g.CreatedAt, &ruleJSON); err != nil {
		return g, err
	}
	g.MembershipRule = parseMembershipRule(ruleJSON)
	return g, nil
}

// parseMembershipRule decodifica el jsonb de membership_rule (ya proyectado a text por
// las consultas). "" ⇒ nil (grupo manual). Un jsonb corrupto ⇒ nil (fail-safe: se
// trata como manual en lectura; el recómputo/edición lo reescribe con una regla válida).
func parseMembershipRule(ruleJSON string) *domain.Rule {
	if ruleJSON == "" {
		return nil
	}
	var rule domain.Rule
	if err := json.Unmarshal([]byte(ruleJSON), &rule); err != nil {
		return nil
	}
	return &rule
}

// GroupWithCount es una fila del directorio admin de grupos: el grupo con su
// recuento de miembros (para GET /admin/groups).
type GroupWithCount struct {
	domain.Group
	MemberCount int
}

// GroupMember es un miembro de un grupo con los datos que expone el admin.
type GroupMember struct {
	UserID string
	Email  string
	Name   string
}

// CreateGroup inserta un grupo en la organización dada. name único por-tenant
// (UNIQUE(org_id,name)) ⇒ 23505 ⇒ ErrConflict. description "" se guarda como NULL.
func (s *Store) CreateGroup(ctx context.Context, q pg.Querier, orgID, name, description string) (domain.Group, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO groups (org_id, name, description)
		VALUES ($1, $2, nullif($3,''))
		RETURNING ` + groupCols
	g, err := scanGroup(q.QueryRow(ctx, sql, orgID, name, description))
	return g, errf("CreateGroup", err)
}

// ListGroups lista los grupos de la org con su member_count, orden estable por
// (name, id). ponytail: sin keyset — los grupos por tenant son decenas, no
// cientos de miles (a diferencia de usuarios); si algún tenant crece a miles,
// paginar por (name,id) como ListOrgUsersPage. member_count por LEFT JOIN +
// GROUP BY sobre la PK de group_members: sin N+1.
func (s *Store) ListGroups(ctx context.Context, orgID string) ([]GroupWithCount, error) {
	const sql = `SELECT g.id, g.org_id, g.name, coalesce(g.description, ''),
		coalesce(g.external_id, ''), g.created_at,
		coalesce(g.membership_rule::text, ''), count(gm.user_id)
		FROM groups g
		LEFT JOIN group_members gm ON gm.group_id = g.id
		WHERE g.org_id = $1
		GROUP BY g.id
		ORDER BY g.name, g.id`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("ListGroups", err)
	}
	defer rows.Close()
	out := make([]GroupWithCount, 0, 16)
	for rows.Next() {
		var gc GroupWithCount
		var ruleJSON string
		if err := rows.Scan(&gc.ID, &gc.OrgID, &gc.Name, &gc.Description,
			&gc.ExternalID, &gc.CreatedAt, &ruleJSON, &gc.MemberCount); err != nil {
			return nil, errf("ListGroups.scan", err)
		}
		gc.MembershipRule = parseMembershipRule(ruleJSON)
		out = append(out, gc)
	}
	return out, errf("ListGroups.iter", rows.Err())
}

// GetGroup recupera un grupo POR (org, id). Es la verja de aislamiento: un id de
// otro tenant no coincide con org_id y da ErrNotFound (404 sin fuga cross-tenant).
func (s *Store) GetGroup(ctx context.Context, orgID, groupID string) (domain.Group, error) {
	const sql = `SELECT ` + groupCols + ` FROM groups WHERE id = $1 AND org_id = $2`
	g, err := scanGroup(s.DB().QueryRow(ctx, sql, groupID, orgID))
	return g, errf("GetGroup", err)
}

// UpdateGroup actualiza nombre y/o descripción de un grupo de la org. El WHERE
// acota por org_id (no toca grupos de otro tenant). ErrNoRows ⇒ ErrNotFound.
func (s *Store) UpdateGroup(ctx context.Context, q pg.Querier, orgID, groupID, name, description string) (domain.Group, error) {
	if s.needsCallIdentityTransaction(q) {
		var out domain.Group
		err := s.InTx(ctx, func(tx pg.Querier) error {
			var err error
			out, err = s.UpdateGroup(ctx, tx, orgID, groupID, name, description)
			return err
		})
		return out, err
	}
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE groups SET name = $3, description = nullif($4,'')
		WHERE id = $1 AND org_id = $2 RETURNING ` + groupCols
	g, err := scanGroup(q.QueryRow(ctx, sql, groupID, orgID, name, description))
	return g, errf("UpdateGroup", err)
}

// DeleteGroup borra un grupo de la org; group_members cae en cascada (FK ON
// DELETE CASCADE, mig 0001). Acotado por org_id. Devuelve true si borró una fila.
func (s *Store) DeleteGroup(ctx context.Context, q pg.Querier, orgID, groupID string) (bool, error) {
	if s.needsCallIdentityTransaction(q) {
		var out bool
		err := s.InTx(ctx, func(tx pg.Querier) error {
			var err error
			out, err = s.DeleteGroup(ctx, tx, orgID, groupID)
			return err
		})
		return out, err
	}
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `DELETE FROM groups WHERE id = $1 AND org_id = $2`, groupID, orgID)
	if err != nil {
		return false, errf("DeleteGroup", err)
	}
	return tag.RowsAffected() > 0, nil
}

// AddGroupMember liga un usuario a un grupo. Defensa en profundidad del
// aislamiento: el INSERT ... SELECT solo procede si el grupo pertenece a la org
// (g.org_id = $3), así ni un bug del handler podría ligar un grupo de otro
// tenant. La pertenencia del usuario a la org la valida el servicio ANTES (para
// devolver 400 claro); aquí la PK (group_id,user_id) da 23505 ⇒ ErrConflict en
// duplicado. RowsAffected 0 sin error ⇒ grupo inexistente en la org ⇒ ErrNotFound.
func (s *Store) AddGroupMember(ctx context.Context, q pg.Querier, orgID, groupID, userID string) error {
	if s.needsCallIdentityTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error { return s.AddGroupMember(ctx, tx, orgID, groupID, userID) })
	}
	if q == nil {
		q = s.DB()
	}
	// membership_rule IS NULL: un grupo DINÁMICO no admite alta manual (su membresía la
	// manda la regla). Gate en el store ⇒ cubre a la vez el CRUD admin y SCIM entrante
	// (defensa en profundidad, choke point único). El handler admin da un 400 claro
	// ANTES; aquí un grupo dinámico simplemente no liga (0 filas ⇒ ErrNotFound).
	const sql = `INSERT INTO group_members (group_id, user_id)
		SELECT $1, $2 WHERE EXISTS (SELECT 1 FROM groups WHERE id = $1 AND org_id = $3 AND membership_rule IS NULL)`
	tag, err := q.Exec(ctx, sql, groupID, userID, orgID)
	if err != nil {
		return errf("AddGroupMember", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// RemoveGroupMember desliga un usuario de un grupo de la org. El EXISTS acota por
// org_id (no toca grupos de otro tenant). Idempotente: 0 filas no es error (el
// servicio ya verificó que el grupo existe en la org con GetGroup).
func (s *Store) RemoveGroupMember(ctx context.Context, q pg.Querier, orgID, groupID, userID string) error {
	if s.needsCallIdentityTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error { return s.RemoveGroupMember(ctx, tx, orgID, groupID, userID) })
	}
	if q == nil {
		q = s.DB()
	}
	// membership_rule IS NULL: no se puede quitar a mano un miembro de un grupo dinámico
	// (idempotente; el gate lo convierte en no-op para grupos con regla).
	const sql = `DELETE FROM group_members
		WHERE group_id = $1 AND user_id = $2
		AND EXISTS (SELECT 1 FROM groups WHERE id = $1 AND org_id = $3 AND membership_rule IS NULL)`
	_, err := q.Exec(ctx, sql, groupID, userID, orgID)
	return errf("RemoveGroupMember", err)
}

// ListGroupMembers lista los miembros de un grupo de la org (email, nombre),
// orden estable por email. El JOIN con groups acotado por org_id aísla el tenant.
func (s *Store) ListGroupMembers(ctx context.Context, orgID, groupID string) ([]GroupMember, error) {
	const sql = `SELECT u.id, u.email, u.name
		FROM group_members gm
		JOIN groups g ON g.id = gm.group_id AND g.org_id = $2
		JOIN users u ON u.id = gm.user_id
		WHERE gm.group_id = $1
		ORDER BY u.email`
	rows, err := s.DB().Query(ctx, sql, groupID, orgID)
	if err != nil {
		return nil, errf("ListGroupMembers", err)
	}
	defer rows.Close()
	out := make([]GroupMember, 0, 16)
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name); err != nil {
			return nil, errf("ListGroupMembers.scan", err)
		}
		out = append(out, m)
	}
	return out, errf("ListGroupMembers.iter", rows.Err())
}

// GroupsForUser devuelve los NOMBRES de los grupos del usuario EN la org dada,
// orden estable por nombre. Es la forma EXACTA que Sytue Drive espera en el claim
// `groups` del JWT: Drive matchea esos strings contra shares.subject (kind=group)
// —campo de texto libre que un humano rellena— así que deben ser los nombres
// legibles, no UUIDs. Acotado por org_id: el claim refleja SOLO los grupos del
// tenant activo (Drive también acota sus shares por tenant). Sirve el índice
// group_members_user_idx (mig 0013) — se ejecuta en CADA emisión de token.
func (s *Store) GroupsForUser(ctx context.Context, orgID, userID string) ([]string, error) {
	const sql = `SELECT g.name
		FROM group_members gm
		JOIN groups g ON g.id = gm.group_id
		WHERE g.org_id = $1 AND gm.user_id = $2
		ORDER BY g.name`
	rows, err := s.DB().Query(ctx, sql, orgID, userID)
	if err != nil {
		return nil, errf("GroupsForUser", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, errf("GroupsForUser.scan", err)
		}
		out = append(out, name)
	}
	return out, errf("GroupsForUser.iter", rows.Err())
}

// --- SCIM 2.0 (RFC 7643 §4.2 · aprovisionamiento entrante de grupos) ----------
// Métodos del recurso Group de SCIM. Reutilizan el mismo aislamiento por org_id
// que el CRUD admin (un group_id/user_id de otro tenant no liga) y comparten con
// él GetGroup, ListGroupMembers, AddGroupMember, RemoveGroupMember y DeleteGroup.
// Añaden lo específico de SCIM: persistir external_id (id del grupo en el IdP
// origen, idempotencia), paginar/filtrar por displayName y reemplazar en bloque
// el conjunto de miembros (PUT / PATCH replace).

// CreateGroupSCIM inserta un grupo con external_id. name único por-tenant
// (UNIQUE(org_id,name)) ⇒ 23505 ⇒ ErrConflict: un reintento del proveedor con el
// mismo displayName da 409, igual que el recurso User existente.
func (s *Store) CreateGroupSCIM(ctx context.Context, q pg.Querier, orgID, name, externalID string) (domain.Group, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO groups (org_id, name, external_id)
		VALUES ($1, $2, nullif($3,''))
		RETURNING ` + groupCols
	g, err := scanGroup(q.QueryRow(ctx, sql, orgID, name, externalID))
	return g, errf("CreateGroupSCIM", err)
}

// UpdateGroupSCIM reemplaza name y external_id de un grupo de la org (PUT / PATCH
// del recurso). Acotado por org_id (no toca grupos de otro tenant). name
// duplicado ⇒ ErrConflict; grupo inexistente en la org ⇒ ErrNotFound.
func (s *Store) UpdateGroupSCIM(ctx context.Context, q pg.Querier, orgID, groupID, name, externalID string) (domain.Group, error) {
	if s.needsCallIdentityTransaction(q) {
		var out domain.Group
		err := s.InTx(ctx, func(tx pg.Querier) error {
			var err error
			out, err = s.UpdateGroupSCIM(ctx, tx, orgID, groupID, name, externalID)
			return err
		})
		return out, err
	}
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE groups SET name = $3, external_id = nullif($4,'')
		WHERE id = $1 AND org_id = $2 RETURNING ` + groupCols
	g, err := scanGroup(q.QueryRow(ctx, sql, groupID, orgID, name, externalID))
	return g, errf("UpdateGroupSCIM", err)
}

// ListGroupsPage pagina los grupos de la org para el listado SCIM (startIndex/
// count ⇒ offset/limit) con filtro opcional por displayName (igualdad exacta,
// `displayName eq`). Devuelve la página y el total. Orden estable (name, id). El
// índice UNIQUE(org_id,name) sirve el WHERE org_id [+ name] y el ORDER BY sin seq
// scan. NO expande miembros (el listado los omite ⇒ sin N+1); GET /{id} los trae
// con ListGroupMembers.
func (s *Store) ListGroupsPage(ctx context.Context, orgID, filterName string, offset, limit int) ([]domain.Group, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args := []any{orgID}
	where := ` WHERE org_id = $1`
	if filterName != "" {
		where += ` AND name = $2`
		args = append(args, filterName)
	}
	var total int
	if err := s.DB().QueryRow(ctx, `SELECT count(*) FROM groups`+where, args...).Scan(&total); err != nil {
		return nil, 0, errf("ListGroupsPage.count", err)
	}
	args = append(args, offset, limit)
	o, l := len(args)-1, len(args)
	sql := `SELECT ` + groupCols + ` FROM groups` + where +
		` ORDER BY name, id OFFSET $` + itoa(o) + ` LIMIT $` + itoa(l)
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, errf("ListGroupsPage", err)
	}
	defer rows.Close()
	out := make([]domain.Group, 0, limit)
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, 0, errf("ListGroupsPage.scan", err)
		}
		out = append(out, g)
	}
	return out, total, errf("ListGroupsPage.iter", rows.Err())
}

// ReplaceGroupMembers fija el conjunto EXACTO de miembros de un grupo de la org
// (PUT del recurso, o PATCH replace de `members`): purga los actuales e inserta
// los userIDs dados en una sola sentencia (unnest, sin N+1), gateada por el grupo
// ∈ org (defensa en profundidad del aislamiento). ON CONFLICT DO NOTHING deduplica
// dentro del conjunto. La pertenencia de cada usuario a la org la valida el
// handler ANTES (400 con feedback claro). Debe correr en tx.
func (s *Store) ReplaceGroupMembers(ctx context.Context, q pg.Querier, orgID, groupID string, userIDs []string) error {
	if s.needsCallIdentityTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error { return s.ReplaceGroupMembers(ctx, tx, orgID, groupID, userIDs) })
	}
	if q == nil {
		q = s.DB()
	}
	// Gate membership_rule IS NULL: SCIM entrante nunca gestiona la membresía de un
	// grupo DINÁMICO (esa la manda la regla). Los grupos dinámicos se crean en la
	// Console, no por SCIM, pero este gate lo blinda igualmente (defensa en profundidad).
	if _, err := q.Exec(ctx, `DELETE FROM group_members
		WHERE group_id = $1 AND EXISTS (SELECT 1 FROM groups WHERE id = $1 AND org_id = $2 AND membership_rule IS NULL)`,
		groupID, orgID); err != nil {
		return errf("ReplaceGroupMembers.delete", err)
	}
	const ins = `INSERT INTO group_members (group_id, user_id)
		SELECT $1, uid::uuid FROM unnest($2::text[]) AS uid
		WHERE EXISTS (SELECT 1 FROM groups WHERE id = $1 AND org_id = $3 AND membership_rule IS NULL)
		ON CONFLICT DO NOTHING`
	if _, err := q.Exec(ctx, ins, groupID, userIDs, orgID); err != nil {
		return errf("ReplaceGroupMembers.insert", err)
	}
	return nil
}
