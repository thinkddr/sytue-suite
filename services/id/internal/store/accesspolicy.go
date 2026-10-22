// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Aislamiento por tenant (docs/CONVENTIONS.md §0.7): igual que grupos/SSO, cada
// método recibe orgID (la org activa del token, resuelto en el handler) y lo cruza
// con access_policies.org_id en CADA query. Una policy o un id de otro tenant no
// liga (ErrNotFound / 0 filas): nunca una fuga cross-tenant. Las columnas uuid[] se
// proyectan como ::text[] para escanearlas en []string (como allowed_email_domains);
// al escribir se pasan []string con cast $n::uuid[] (los UUID los valida el handler).

// policyCols proyecta una access_policy. days_mask/start/end se castean a int para
// escanearlos en int (columnas smallint).
const policyCols = `id, org_id, name, enabled, priority, action,
	include_users::text[], include_groups::text[], exclude_users::text[], exclude_groups::text[],
	ip_ranges, ip_negate, client_ids, require_trusted_device,
	days_mask::int, start_minute::int, end_minute::int,
	coalesce(created_by::text,''), created_at, updated_at`

func scanPolicy(row pgx.Row) (domain.AccessPolicy, error) {
	var p domain.AccessPolicy
	var action string
	err := row.Scan(&p.ID, &p.OrgID, &p.Name, &p.Enabled, &p.Priority, &action,
		&p.IncludeUsers, &p.IncludeGroups, &p.ExcludeUsers, &p.ExcludeGroups,
		&p.IPRanges, &p.IPNegate, &p.ClientIDs, &p.RequireTrustedDevice,
		&p.DaysMask, &p.StartMinute, &p.EndMinute,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return domain.AccessPolicy{}, err
	}
	p.Action = domain.AccessAction(action)
	return p, nil
}

// nonNilStrs evita insertar NULL en una columna array NOT NULL: un slice nil se
// guarda como '{}' (sin restricción en esa dimensión).
func nonNilStrs(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ListEnabledPolicies devuelve las políticas HABILITADAS de una org, ordenadas por
// prioridad — la carga que hace el motor en CADA login. Sin políticas ⇒ slice vacío
// (el motor traduce eso en "permitir": anti-lockout). Sirve access_policies_org_idx.
func (s *Store) ListEnabledPolicies(ctx context.Context, orgID string) ([]domain.AccessPolicy, error) {
	const sql = `SELECT ` + policyCols + ` FROM access_policies
		WHERE org_id = $1 AND enabled ORDER BY priority, id`
	return s.queryPolicies(ctx, sql, orgID)
}

// ListAccessPolicies lista TODAS las políticas de una org (Console admin), incluidas
// las deshabilitadas, por prioridad. Los tenants tienen decenas de políticas, no
// cientos de miles: sin keyset (como ListGroups).
func (s *Store) ListAccessPolicies(ctx context.Context, orgID string) ([]domain.AccessPolicy, error) {
	const sql = `SELECT ` + policyCols + ` FROM access_policies
		WHERE org_id = $1 ORDER BY priority, id`
	return s.queryPolicies(ctx, sql, orgID)
}

func (s *Store) queryPolicies(ctx context.Context, sql, orgID string) ([]domain.AccessPolicy, error) {
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("queryPolicies", err)
	}
	defer rows.Close()
	out := make([]domain.AccessPolicy, 0, 16)
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, errf("queryPolicies.scan", err)
		}
		out = append(out, p)
	}
	return out, errf("queryPolicies.iter", rows.Err())
}

// GetAccessPolicy recupera una política POR (org, id). Es la verja de aislamiento: un
// id de otro tenant no coincide con org_id y da ErrNotFound (404 sin fuga cross-tenant).
func (s *Store) GetAccessPolicy(ctx context.Context, orgID, id string) (domain.AccessPolicy, error) {
	const sql = `SELECT ` + policyCols + ` FROM access_policies WHERE id = $1 AND org_id = $2`
	p, err := scanPolicy(s.DB().QueryRow(ctx, sql, id, orgID))
	return p, errf("GetAccessPolicy", err)
}

// CreateAccessPolicy inserta una política en la org dada.
func (s *Store) CreateAccessPolicy(ctx context.Context, q pg.Querier, p domain.AccessPolicy) (domain.AccessPolicy, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO access_policies
		(org_id, name, enabled, priority, action,
		 include_users, include_groups, exclude_users, exclude_groups,
		 ip_ranges, ip_negate, client_ids, require_trusted_device,
		 days_mask, start_minute, end_minute, created_by)
		VALUES ($1,$2,$3,$4,$5,
		        $6::uuid[],$7::uuid[],$8::uuid[],$9::uuid[],
		        $10,$11,$12,$13,$14,$15,$16, nullif($17,'')::uuid)
		RETURNING ` + policyCols
	row := q.QueryRow(ctx, sql, p.OrgID, p.Name, p.Enabled, p.Priority, string(p.Action),
		nonNilStrs(p.IncludeUsers), nonNilStrs(p.IncludeGroups),
		nonNilStrs(p.ExcludeUsers), nonNilStrs(p.ExcludeGroups),
		nonNilStrs(p.IPRanges), p.IPNegate, nonNilStrs(p.ClientIDs), p.RequireTrustedDevice,
		p.DaysMask, p.StartMinute, p.EndMinute, p.CreatedBy)
	got, err := scanPolicy(row)
	return got, errf("CreateAccessPolicy", err)
}

// UpdateAccessPolicy actualiza una política acotada por (org, id): una de otra org NO
// se toca (aislamiento tenant). ErrNotFound si no pertenece a la org.
func (s *Store) UpdateAccessPolicy(ctx context.Context, q pg.Querier, p domain.AccessPolicy) (domain.AccessPolicy, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE access_policies SET
		name=$3, enabled=$4, priority=$5, action=$6,
		include_users=$7::uuid[], include_groups=$8::uuid[],
		exclude_users=$9::uuid[], exclude_groups=$10::uuid[],
		ip_ranges=$11, ip_negate=$12, client_ids=$13, require_trusted_device=$14,
		days_mask=$15, start_minute=$16, end_minute=$17, updated_at=now()
		WHERE id=$1 AND org_id=$2
		RETURNING ` + policyCols
	row := q.QueryRow(ctx, sql, p.ID, p.OrgID, p.Name, p.Enabled, p.Priority, string(p.Action),
		nonNilStrs(p.IncludeUsers), nonNilStrs(p.IncludeGroups),
		nonNilStrs(p.ExcludeUsers), nonNilStrs(p.ExcludeGroups),
		nonNilStrs(p.IPRanges), p.IPNegate, nonNilStrs(p.ClientIDs), p.RequireTrustedDevice,
		p.DaysMask, p.StartMinute, p.EndMinute)
	got, err := scanPolicy(row)
	return got, errf("UpdateAccessPolicy", err)
}

// DeleteAccessPolicy borra una política acotada por (org, id). Devuelve true si borró
// una fila (existía y pertenecía a la org).
func (s *Store) DeleteAccessPolicy(ctx context.Context, orgID, id string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM access_policies WHERE id=$1 AND org_id=$2`, id, orgID)
	if err != nil {
		return false, errf("DeleteAccessPolicy", err)
	}
	return tag.RowsAffected() > 0, nil
}

// GroupIDsForUser devuelve los UUID de los grupos del usuario EN la org dada — la
// forma que necesita el motor de Acceso Condicional para casar include/exclude por
// grupo (a diferencia de GroupsForUser, que devuelve los NOMBRES para el claim del
// JWT). Acotado por org_id; sirve group_members_user_idx (mig 0013).
func (s *Store) GroupIDsForUser(ctx context.Context, orgID, userID string) ([]string, error) {
	const sql = `SELECT g.id::text
		FROM group_members gm
		JOIN groups g ON g.id = gm.group_id
		WHERE g.org_id = $1 AND gm.user_id = $2`
	rows, err := s.DB().Query(ctx, sql, orgID, userID)
	if err != nil {
		return nil, errf("GroupIDsForUser", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, errf("GroupIDsForUser.scan", err)
		}
		out = append(out, id)
	}
	return out, errf("GroupIDsForUser.iter", rows.Err())
}
