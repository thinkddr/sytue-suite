// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Roles personalizados / admin delegada (mig 0019). Mismo aislamiento por tenant que
// grupos/access_policies (docs/CONVENTIONS.md §0.7): cada método recibe orgID (la org
// activa del token, resuelto en el handler) y lo cruza con custom_roles.org_id en CADA
// query — un role_id o user_id de otro tenant no liga (ErrNotFound / 0 filas), nunca una
// fuga cross-tenant. `permissions` es text[]: pgx lo escanea/escribe como []string.

const roleCols = `id, org_id, name, permissions, created_at, updated_at`

func scanRole(row pgx.Row) (domain.CustomRole, error) {
	var r domain.CustomRole
	err := row.Scan(&r.ID, &r.OrgID, &r.Name, &r.Permissions, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

// CustomRoleWithCount es una fila del listado admin: el rol con su recuento de usuarios
// asignados (para GET /admin/roles).
type CustomRoleWithCount struct {
	domain.CustomRole
	MemberCount int
}

// RoleMember es un usuario asignado a un rol, con los datos que expone el admin.
type RoleMember struct {
	UserID string
	Email  string
	Name   string
}

// ListCustomRoles lista los roles de la org con su member_count, orden estable por
// (name, id). Como los grupos, los roles por tenant son decenas: sin keyset. member_count
// por LEFT JOIN + GROUP BY sobre la PK de user_custom_roles (sin N+1).
func (s *Store) ListCustomRoles(ctx context.Context, orgID string) ([]CustomRoleWithCount, error) {
	const sql = `SELECT cr.id, cr.org_id, cr.name, cr.permissions, cr.created_at, cr.updated_at,
		count(ucr.user_id)
		FROM custom_roles cr
		LEFT JOIN user_custom_roles ucr ON ucr.role_id = cr.id
		WHERE cr.org_id = $1
		GROUP BY cr.id
		ORDER BY cr.name, cr.id`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("ListCustomRoles", err)
	}
	defer rows.Close()
	out := make([]CustomRoleWithCount, 0, 16)
	for rows.Next() {
		var rc CustomRoleWithCount
		if err := rows.Scan(&rc.ID, &rc.OrgID, &rc.Name, &rc.Permissions,
			&rc.CreatedAt, &rc.UpdatedAt, &rc.MemberCount); err != nil {
			return nil, errf("ListCustomRoles.scan", err)
		}
		out = append(out, rc)
	}
	return out, errf("ListCustomRoles.iter", rows.Err())
}

// CustomRolesForUser son los roles personalizados ASIGNADOS a un usuario en la
// org, con sus permisos. PermissionsForUserInOrg ya devuelve la unión aplanada —
// lo que necesita el claim `perms`—, pero el alcance efectivo tiene que decir de
// QUÉ rol sale cada permiso, y eso se pierde al aplanar. Una consulta, sin N+1.
func (s *Store) CustomRolesForUser(ctx context.Context, orgID, userID string) ([]domain.CustomRole, error) {
	const sql = `SELECT ` + roleCols + ` FROM custom_roles cr
		JOIN user_custom_roles ucr ON ucr.role_id = cr.id
		WHERE cr.org_id = $1 AND ucr.org_id = $1 AND ucr.user_id = $2
		ORDER BY cr.name, cr.id`
	rows, err := s.DB().Query(ctx, sql, orgID, userID)
	if err != nil {
		return nil, errf("CustomRolesForUser", err)
	}
	defer rows.Close()
	out := make([]domain.CustomRole, 0, 8)
	for rows.Next() {
		r, err := scanRole(rows)
		if err != nil {
			return nil, errf("CustomRolesForUser.scan", err)
		}
		out = append(out, r)
	}
	return out, errf("CustomRolesForUser.iter", rows.Err())
}

// GetCustomRole recupera un rol POR (org, id). Verja de aislamiento: un id de otro
// tenant no coincide con org_id y da ErrNotFound (404 sin fuga cross-tenant).
func (s *Store) GetCustomRole(ctx context.Context, orgID, id string) (domain.CustomRole, error) {
	const sql = `SELECT ` + roleCols + ` FROM custom_roles WHERE id = $1 AND org_id = $2`
	r, err := scanRole(s.DB().QueryRow(ctx, sql, id, orgID))
	return r, errf("GetCustomRole", err)
}

// CreateCustomRole inserta un rol en la org. name único por-tenant (UNIQUE(org_id,name))
// ⇒ 23505 ⇒ ErrConflict. perms ya validado/normalizado por el handler.
func (s *Store) CreateCustomRole(ctx context.Context, q pg.Querier, orgID, name string, perms []string) (domain.CustomRole, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO custom_roles (org_id, name, permissions)
		VALUES ($1, $2, $3) RETURNING ` + roleCols
	r, err := scanRole(q.QueryRow(ctx, sql, orgID, name, nonNilStrs(perms)))
	return r, errf("CreateCustomRole", err)
}

// UpdateCustomRole reemplaza name y permissions de un rol de la org. Acotado por org_id
// (no toca roles de otro tenant). ErrNoRows ⇒ ErrNotFound; name duplicado ⇒ ErrConflict.
func (s *Store) UpdateCustomRole(ctx context.Context, q pg.Querier, orgID, id, name string, perms []string) (domain.CustomRole, error) {
	if s.needsCallIdentityTransaction(q) {
		var out domain.CustomRole
		err := s.InTx(ctx, func(tx pg.Querier) error {
			var err error
			out, err = s.UpdateCustomRole(ctx, tx, orgID, id, name, perms)
			return err
		})
		return out, err
	}
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE custom_roles SET name = $3, permissions = $4, updated_at = now()
		WHERE id = $1 AND org_id = $2 RETURNING ` + roleCols
	r, err := scanRole(q.QueryRow(ctx, sql, id, orgID, name, nonNilStrs(perms)))
	return r, errf("UpdateCustomRole", err)
}

// DeleteCustomRole borra un rol de la org; sus asignaciones caen en cascada (FK).
// Acotado por org_id. Devuelve true si borró una fila (existía en la org).
func (s *Store) DeleteCustomRole(ctx context.Context, orgID, id string) (bool, error) {
	var count int64
	err := s.InTx(ctx, func(q pg.Querier) error {
		tag, err := q.Exec(ctx, `DELETE FROM custom_roles WHERE id=$1 AND org_id=$2`, id, orgID)
		count = tag.RowsAffected()
		return err
	})
	if err != nil {
		return false, errf("DeleteCustomRole", err)
	}
	return count > 0, nil
}

// AssignCustomRole liga un usuario a un rol. Defensa en profundidad del aislamiento: el
// INSERT ... SELECT solo procede si el rol pertenece a la org, y persiste org_id del rol
// (consistencia con user_custom_roles.org_id). La pertenencia del usuario a la org la
// valida el handler ANTES (400 claro). Duplicado ⇒ PK 23505 ⇒ ErrConflict. RowsAffected
// 0 sin error ⇒ rol inexistente en la org ⇒ ErrNotFound.
func (s *Store) AssignCustomRole(ctx context.Context, q pg.Querier, orgID, roleID, userID string) error {
	if s.needsCallIdentityTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error { return s.AssignCustomRole(ctx, tx, orgID, roleID, userID) })
	}
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO user_custom_roles (org_id, role_id, user_id)
		SELECT org_id, id, $2 FROM custom_roles WHERE id = $1 AND org_id = $3`
	tag, err := q.Exec(ctx, sql, roleID, userID, orgID)
	if err != nil {
		return errf("AssignCustomRole", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UnassignCustomRole desliga un usuario de un rol de la org. El EXISTS acota por org_id.
// Idempotente: 0 filas no es error (el handler ya verificó que el rol existe en la org).
func (s *Store) UnassignCustomRole(ctx context.Context, q pg.Querier, orgID, roleID, userID string) error {
	if s.needsCallIdentityTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error { return s.UnassignCustomRole(ctx, tx, orgID, roleID, userID) })
	}
	if q == nil {
		q = s.DB()
	}
	const sql = `DELETE FROM user_custom_roles
		WHERE role_id = $1 AND user_id = $2
		AND EXISTS (SELECT 1 FROM custom_roles WHERE id = $1 AND org_id = $3)`
	_, err := q.Exec(ctx, sql, roleID, userID, orgID)
	return errf("UnassignCustomRole", err)
}

// ListCustomRoleMembers lista los usuarios asignados a un rol de la org (email, nombre),
// orden estable por email. El JOIN acotado por org_id aísla el tenant.
func (s *Store) ListCustomRoleMembers(ctx context.Context, orgID, roleID string) ([]RoleMember, error) {
	const sql = `SELECT u.id, u.email, u.name
		FROM user_custom_roles ucr
		JOIN custom_roles cr ON cr.id = ucr.role_id AND cr.org_id = $2
		JOIN users u ON u.id = ucr.user_id
		WHERE ucr.role_id = $1
		ORDER BY u.email`
	rows, err := s.DB().Query(ctx, sql, roleID, orgID)
	if err != nil {
		return nil, errf("ListCustomRoleMembers", err)
	}
	defer rows.Close()
	out := make([]RoleMember, 0, 16)
	for rows.Next() {
		var m RoleMember
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name); err != nil {
			return nil, errf("ListCustomRoleMembers.scan", err)
		}
		out = append(out, m)
	}
	return out, errf("ListCustomRoleMembers.iter", rows.Err())
}

// PermissionsForUserInOrg devuelve los permisos EFECTIVOS del usuario en la org: la
// unión (distinct) de los permisos de todos los roles personalizados que tiene
// asignados. Es la fuente del claim `perms` del JWT — se ejecuta en CADA emisión de
// token, como GroupsForUser. Acotado por org_id (sirve user_custom_roles_user_idx). Sin
// roles asignados ⇒ slice vacío (deny-by-default; owner/admin no lo necesitan).
func (s *Store) PermissionsForUserInOrg(ctx context.Context, orgID, userID string) ([]string, error) {
	const sql = `SELECT DISTINCT p
		FROM user_custom_roles ucr
		JOIN custom_roles cr ON cr.id = ucr.role_id
		CROSS JOIN LATERAL unnest(cr.permissions) AS p
		WHERE ucr.org_id = $1 AND ucr.user_id = $2
		ORDER BY p`
	rows, err := s.DB().Query(ctx, sql, orgID, userID)
	if err != nil {
		return nil, errf("PermissionsForUserInOrg", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, errf("PermissionsForUserInOrg.scan", err)
		}
		out = append(out, p)
	}
	return out, errf("PermissionsForUserInOrg.iter", rows.Err())
}
