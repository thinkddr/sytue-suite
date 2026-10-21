// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Materialización de la membresía de grupos DINÁMICOS (mig 0021). ELECCIÓN DE DISEÑO:
// materializamos en group_members (misma tabla que la membresía manual) en vez de
// evaluar on-read en GroupsForUser. Motivo: TODOS los lectores existentes —
// GroupsForUser (→ claim `groups` del JWT → SCIM saliente), ListGroupMembers, el
// member_count de ListGroups— leen group_members y así reflejan la membresía dinámica
// SIN cambio alguno; y no cargamos el camino caliente de emisión de token con evaluación
// de reglas. El discriminador es a nivel de GRUPO (groups.membership_rule IS NOT NULL):
// un grupo es dinámico O manual, nunca mezcla, luego no hace falta marca por-fila.
//
// Tres disparadores mantienen la materialización al día:
//   - RecomputeDynamicGroup: al crear/editar la regla → recomputa TODO el grupo.
//   - ReevaluateUserDynamicGroups: al cambiar los atributos de un usuario (perfil admin
//     o SCIM entrante) → reevalúa solo a ESE usuario contra los grupos dinámicos de la
//     org (acotado, sin N+1).
//   - RecomputeOrgDynamicGroups / RecomputeAllDynamicGroups: recálculo COMPLETO contra el
//     estado actual, a petición (botón «Recalcular ahora») o por el barrido periódico del
//     servicio. Repara la deriva: la regla creada ANTES que los usuarios, el atributo
//     cambiado por una vía que no reevalúa, o el usuario dado de baja.
//
// Las escrituras son DIFERENCIALES (syncDynamicMembers): en estado estable el recálculo
// no escribe nada, lo que hace barato repetirlo cada pocos minutos en cada réplica.

// SetGroupRule marca un grupo como DINÁMICO fijando (o actualizando) su membership_rule.
// Acotado por org_id (no toca grupos de otro tenant). ErrNoRows ⇒ ErrNotFound.
func (s *Store) SetGroupRule(ctx context.Context, q pg.Querier, orgID, groupID string, rule domain.Rule) (domain.Group, error) {
	if s.needsCallIdentityTransaction(q) {
		var out domain.Group
		err := s.InTx(ctx, func(tx pg.Querier) error {
			var err error
			out, err = s.SetGroupRule(ctx, tx, orgID, groupID, rule)
			return err
		})
		return out, err
	}
	if q == nil {
		q = s.DB()
	}
	raw, err := json.Marshal(rule)
	if err != nil {
		return domain.Group{}, errf("SetGroupRule.marshal", err)
	}
	// $3::jsonb: pasamos el JSON como texto y Postgres lo castea — evita ambigüedades
	// del códec de pgx con jsonb.
	const sql = `UPDATE groups SET membership_rule = $3::jsonb
		WHERE id = $1 AND org_id = $2 RETURNING ` + groupCols
	g, err := scanGroup(q.QueryRow(ctx, sql, groupID, orgID, string(raw)))
	return g, errf("SetGroupRule", err)
}

// orgUser es un usuario de la org con los atributos que evalúan las reglas.
type orgUser struct {
	id, email, name, department, title, location string
}

// RecomputeResult resume el efecto de un recálculo sobre UN grupo dinámico: cuántos
// ENTRARON, cuántos SALIERON y cuántos quedan. La Console lo enseña tal cual tras
// «Recalcular ahora» (números reales, no un «hecho» genérico).
type RecomputeResult struct {
	GroupID string
	Added   int
	Removed int
	Total   int
}

// SweepResult resume un barrido completo de grupos dinámicos (todas las orgs).
type SweepResult struct {
	Orgs    int
	Groups  int
	Added   int
	Removed int
}

// loadOrgUserAttrs carga en UNA consulta a TODOS los usuarios (no borrados) CON membresía
// en la org junto con los atributos que evalúan las reglas, en orden estable por email.
//
// ponytail: carga todos los miembros de la org (id + 5 atributos cortos) por recálculo.
// Acotado por el tamaño de la org, y se hace UNA vez por org aunque tenga K reglas. Techo:
// si una org llega a cientos de miles de usuarios, paginar por keyset (created_at,id) y
// acumular matches por lotes; EvaluateRule ya es O(nº condiciones) por usuario.
func (s *Store) loadOrgUserAttrs(ctx context.Context, q pg.Querier, orgID string) ([]orgUser, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `SELECT u.id, u.email, u.name,
		coalesce(u.department, ''), coalesce(u.title, ''), coalesce(u.location, '')
		FROM users u
		JOIN memberships m ON m.user_id = u.id AND m.org_id = $1
		WHERE u.status <> 'deleted'
		ORDER BY u.email`
	rows, err := q.Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("loadOrgUserAttrs", err)
	}
	defer rows.Close()
	var out []orgUser
	for rows.Next() {
		var u orgUser
		if err := rows.Scan(&u.id, &u.email, &u.name, &u.department, &u.title, &u.location); err != nil {
			return nil, errf("loadOrgUserAttrs.scan", err)
		}
		out = append(out, u)
	}
	return out, errf("loadOrgUserAttrs.iter", rows.Err())
}

// matchRule devuelve, de los usuarios ya cargados, los que CUMPLEN la regla. Reutiliza un
// único mapa de atributos entre iteraciones (EvaluateRule no lo retiene).
func matchRule(users []orgUser, rule domain.Rule) []GroupMember {
	attrs := make(map[string]string, len(domain.RuleAttributes))
	var out []GroupMember
	for _, u := range users {
		attrs[domain.AttrEmail] = u.email
		attrs[domain.AttrName] = u.name
		attrs[domain.AttrDepartment] = u.department
		attrs[domain.AttrTitle] = u.title
		attrs[domain.AttrLocation] = u.location
		if domain.EvaluateRule(rule, attrs) {
			out = append(out, GroupMember{UserID: u.id, Email: u.email, Name: u.name})
		}
	}
	return out
}

// evalOrgUsers carga los usuarios de la org y devuelve los que cumplen la regla. Sin N+1:
// una query + evaluación en memoria.
func (s *Store) evalOrgUsers(ctx context.Context, q pg.Querier, orgID string, rule domain.Rule) ([]GroupMember, error) {
	users, err := s.loadOrgUserAttrs(ctx, q, orgID)
	if err != nil {
		return nil, err
	}
	return matchRule(users, rule), nil
}

// dynamicMemberIDs devuelve los ids ya materializados de un grupo DINÁMICO de la org
// (vacío si el grupo es manual o de otro tenant: mismo gate que las escrituras).
func (s *Store) dynamicMemberIDs(ctx context.Context, q pg.Querier, orgID, groupID string) (map[string]struct{}, error) {
	const sql = `SELECT gm.user_id FROM group_members gm
		JOIN groups g ON g.id = gm.group_id AND g.org_id = $2 AND g.membership_rule IS NOT NULL
		WHERE gm.group_id = $1`
	rows, err := q.Query(ctx, sql, groupID, orgID)
	if err != nil {
		return nil, errf("dynamicMemberIDs", err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, errf("dynamicMemberIDs.scan", err)
		}
		out[id] = struct{}{}
	}
	return out, errf("dynamicMemberIDs.iter", rows.Err())
}

// syncDynamicMembers ajusta la membresía materializada de un grupo DINÁMICO al conjunto
// EXACTO `want`, escribiendo SOLO LA DIFERENCIA: inserta los que faltan y borra los que
// sobran, cada uno en una sentencia (unnest, sin N+1). Dos motivos frente a un
// purgar-e-insertar: en estado estable NO escribe nada (lo que hace barato el barrido
// periódico) y nunca existe el instante en que el grupo está vacío — un token emitido en
// esa ventana perdería el claim `groups`. Gate membership_rule IS NOT NULL + org_id en
// cada sentencia: jamás toca un grupo manual ni uno de otro tenant. Debe correr en tx.
func (s *Store) syncDynamicMembers(ctx context.Context, q pg.Querier, orgID, groupID string, want []string) (added, removed int, err error) {
	if q == nil {
		q = s.DB()
	}
	current, err := s.dynamicMemberIDs(ctx, q, orgID, groupID)
	if err != nil {
		return 0, 0, err
	}
	wantSet := make(map[string]struct{}, len(want))
	var toAdd []string
	for _, id := range want {
		wantSet[id] = struct{}{}
		if _, ok := current[id]; !ok {
			toAdd = append(toAdd, id)
		}
	}
	var toDel []string
	for id := range current {
		if _, ok := wantSet[id]; !ok {
			toDel = append(toDel, id)
		}
	}
	if len(toAdd) > 0 {
		const ins = `INSERT INTO group_members (group_id, user_id)
			SELECT $1, uid::uuid FROM unnest($2::text[]) AS uid
			WHERE EXISTS (SELECT 1 FROM groups WHERE id = $1 AND org_id = $3 AND membership_rule IS NOT NULL)
			ON CONFLICT DO NOTHING`
		tag, e := q.Exec(ctx, ins, groupID, toAdd, orgID)
		if e != nil {
			return 0, 0, errf("syncDynamicMembers.insert", e)
		}
		added = int(tag.RowsAffected())
	}
	if len(toDel) > 0 {
		const del = `DELETE FROM group_members
			WHERE group_id = $1 AND user_id IN (SELECT uid::uuid FROM unnest($2::text[]) AS uid)
			AND EXISTS (SELECT 1 FROM groups WHERE id = $1 AND org_id = $3 AND membership_rule IS NOT NULL)`
		tag, e := q.Exec(ctx, del, groupID, toDel, orgID)
		if e != nil {
			return 0, 0, errf("syncDynamicMembers.delete", e)
		}
		removed = int(tag.RowsAffected())
	}
	return added, removed, nil
}

// applyMatch materializa el resultado de evaluar una regla y devuelve su efecto.
func (s *Store) applyMatch(ctx context.Context, q pg.Querier, orgID, groupID string, matched []GroupMember) (RecomputeResult, error) {
	ids := make([]string, len(matched))
	for i, m := range matched {
		ids[i] = m.UserID
	}
	added, removed, err := s.syncDynamicMembers(ctx, q, orgID, groupID, ids)
	if err != nil {
		return RecomputeResult{}, err
	}
	return RecomputeResult{GroupID: groupID, Added: added, Removed: removed, Total: len(ids)}, nil
}

// RecomputeDynamicGroup recomputa la membresía COMPLETA de un grupo dinámico a partir de
// su regla y la materializa en group_members. Corre en la tx dada (o en la suya). Se
// invoca al crear/editar la regla y desde «Recalcular ahora».
func (s *Store) RecomputeDynamicGroup(ctx context.Context, q pg.Querier, orgID, groupID string, rule domain.Rule) (RecomputeResult, error) {
	if s.needsCallIdentityTransaction(q) {
		var out RecomputeResult
		err := s.InTx(ctx, func(tx pg.Querier) error {
			var err error
			out, err = s.RecomputeDynamicGroup(ctx, tx, orgID, groupID, rule)
			return err
		})
		return out, err
	}
	matched, err := s.evalOrgUsers(ctx, q, orgID, rule)
	if err != nil {
		return RecomputeResult{}, err
	}
	return s.applyMatch(ctx, q, orgID, groupID, matched)
}

// RecomputeOrgDynamicGroups recomputa TODOS los grupos dinámicos de una org contra el
// estado ACTUAL de sus usuarios y devuelve el efecto por grupo. Sin N+1: carga los
// usuarios de la org UNA sola vez y evalúa las K reglas en memoria (K = decenas), todo en
// UNA tx para que ningún lector vea una membresía a medias.
func (s *Store) RecomputeOrgDynamicGroups(ctx context.Context, orgID string) ([]RecomputeResult, error) {
	var out []RecomputeResult
	err := s.InTx(ctx, func(q pg.Querier) error {
		groups, gErr := s.listDynamicGroups(ctx, q, orgID)
		if gErr != nil || len(groups) == 0 {
			return gErr
		}
		users, uErr := s.loadOrgUserAttrs(ctx, q, orgID)
		if uErr != nil {
			return uErr
		}
		out = make([]RecomputeResult, 0, len(groups))
		for _, g := range groups {
			res, e := s.applyMatch(ctx, q, orgID, g.id, matchRule(users, g.rule))
			if e != nil {
				return e
			}
			out = append(out, res)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RecomputeAllDynamicGroups recomputa los grupos dinámicos de TODAS las organizaciones;
// lo llama el barrido periódico del servicio (cmd/id/main.go).
//
// Enumerar cross-org es seguro AQUÍ: el esquema de Sytue ID no lleva RLS (su aislamiento
// es por org_id en cada consulta, ver mig 0021), a diferencia de los barridos de otros
// servicios de la suite. Cada org se recompute en su PROPIA tx: una org que falle no
// aborta a las ya hechas.
//
// ponytail: sin lock distribuido — N réplicas pueden barrer a la vez. Las escrituras son
// diferenciales e idempotentes (INSERT ... ON CONFLICT DO NOTHING / DELETE acotado), así
// que el peor caso es trabajo repetido. Techo: si el nº de réplicas × orgs lo hace notar,
// un pg_try_advisory_lock por org al entrar.
func (s *Store) RecomputeAllDynamicGroups(ctx context.Context) (SweepResult, error) {
	rows, err := s.DB().Query(ctx, `SELECT DISTINCT org_id FROM groups WHERE membership_rule IS NOT NULL`)
	if err != nil {
		return SweepResult{}, errf("RecomputeAllDynamicGroups", err)
	}
	var orgIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return SweepResult{}, errf("RecomputeAllDynamicGroups.scan", err)
		}
		orgIDs = append(orgIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return SweepResult{}, errf("RecomputeAllDynamicGroups.iter", err)
	}
	var sum SweepResult
	for _, orgID := range orgIDs {
		res, err := s.RecomputeOrgDynamicGroups(ctx, orgID)
		if err != nil {
			return sum, err
		}
		sum.Orgs++
		sum.Groups += len(res)
		for _, r := range res {
			sum.Added += r.Added
			sum.Removed += r.Removed
		}
	}
	return sum, nil
}

// PreviewDynamicMembers evalúa una regla contra la org SIN persistir: devuelve una
// muestra (hasta limit, orden por email) y el total de coincidencias. Alimenta la
// previsualización de la Console antes de guardar.
func (s *Store) PreviewDynamicMembers(ctx context.Context, orgID string, rule domain.Rule, limit int) ([]GroupMember, int, error) {
	matched, err := s.evalOrgUsers(ctx, nil, orgID, rule)
	if err != nil {
		return nil, 0, err
	}
	total := len(matched)
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, total, nil
}

// ReevaluateUserDynamicGroups reevalúa a UN usuario contra TODOS los grupos dinámicos de
// la org tras cambiar sus atributos: lo añade donde ahora cumple y lo quita donde ya no.
// Acotado (un usuario × K grupos dinámicos, K = decenas), sin N+1 sobre usuarios. Se
// llama en la misma tx que el cambio de atributos.
//
// ponytail: reevalúa SOLO la org en cuyo contexto cambió el atributo (activeOrg del
// admin / org ligada de SCIM). Un usuario en varias orgs verá refrescados los grupos
// dinámicos de las OTRAS cuando allí se reedite una regla o se le toque de nuevo. Techo:
// si multi-org con atributos compartidos importa, iterar sus memberships aquí.
func (s *Store) ReevaluateUserDynamicGroups(ctx context.Context, q pg.Querier, orgID, userID string) error {
	if s.needsCallIdentityTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error { return s.ReevaluateUserDynamicGroups(ctx, tx, orgID, userID) })
	}
	if q == nil {
		q = s.DB()
	}
	// Atributos del usuario (leídos por q para ver los recién escritos en la misma tx).
	var email, name, dept, title, loc string
	if err := q.QueryRow(ctx, `SELECT email, name, coalesce(department,''), coalesce(title,''), coalesce(location,'')
		FROM users WHERE id = $1`, userID).Scan(&email, &name, &dept, &title, &loc); err != nil {
		return errf("ReevaluateUserDynamicGroups.user", err)
	}
	groups, err := s.listDynamicGroups(ctx, q, orgID)
	if err != nil {
		return err
	}
	attrs := map[string]string{
		domain.AttrEmail:      email,
		domain.AttrName:       name,
		domain.AttrDepartment: dept,
		domain.AttrTitle:      title,
		domain.AttrLocation:   loc,
	}
	for _, g := range groups {
		var err error
		if domain.EvaluateRule(g.rule, attrs) {
			err = s.addDynamicMember(ctx, q, orgID, g.id, userID)
		} else {
			err = s.removeDynamicMember(ctx, q, orgID, g.id, userID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// dynGroup es un grupo dinámico con su regla ya parseada.
type dynGroup struct {
	id   string
	rule domain.Rule
}

// listDynamicGroups devuelve los grupos DINÁMICOS de la org con su regla parseada. Los
// acumula y CIERRA el cursor antes de devolver (una conexión no admite consultas
// solapadas en tx, y quien llama emite INSERT/DELETE a continuación). Una regla ilegible
// se ignora: un grupo con jsonb corrupto no debe tumbar el recálculo de los demás.
func (s *Store) listDynamicGroups(ctx context.Context, q pg.Querier, orgID string) ([]dynGroup, error) {
	if q == nil {
		q = s.DB()
	}
	rows, err := q.Query(ctx, `SELECT id, coalesce(membership_rule::text, '')
		FROM groups WHERE org_id = $1 AND membership_rule IS NOT NULL ORDER BY name`, orgID)
	if err != nil {
		return nil, errf("listDynamicGroups", err)
	}
	var out []dynGroup
	for rows.Next() {
		var id, ruleJSON string
		if err := rows.Scan(&id, &ruleJSON); err != nil {
			rows.Close()
			return nil, errf("listDynamicGroups.scan", err)
		}
		if r := parseMembershipRule(ruleJSON); r != nil {
			out = append(out, dynGroup{id: id, rule: *r})
		}
	}
	rows.Close()
	return out, errf("listDynamicGroups.iter", rows.Err())
}

// addDynamicMember liga a un usuario a un grupo dinámico si (a) el grupo tiene regla y es
// de la org y (b) el usuario es miembro de la org. Idempotente (ON CONFLICT DO NOTHING).
func (s *Store) addDynamicMember(ctx context.Context, q pg.Querier, orgID, groupID, userID string) error {
	const sql = `INSERT INTO group_members (group_id, user_id)
		SELECT $1, $2
		WHERE EXISTS (SELECT 1 FROM groups WHERE id = $1 AND org_id = $3 AND membership_rule IS NOT NULL)
		  AND EXISTS (SELECT 1 FROM memberships WHERE org_id = $3 AND user_id = $2)
		ON CONFLICT DO NOTHING`
	_, err := q.Exec(ctx, sql, groupID, userID, orgID)
	return errf("addDynamicMember", err)
}

// removeDynamicMember desliga a un usuario de un grupo dinámico de la org (idempotente).
func (s *Store) removeDynamicMember(ctx context.Context, q pg.Querier, orgID, groupID, userID string) error {
	const sql = `DELETE FROM group_members
		WHERE group_id = $1 AND user_id = $2
		AND EXISTS (SELECT 1 FROM groups WHERE id = $1 AND org_id = $3 AND membership_rule IS NOT NULL)`
	_, err := q.Exec(ctx, sql, groupID, userID, orgID)
	return errf("removeDynamicMember", err)
}

// SetUserAttributes fija SOLO los atributos de perfil que alimentan las reglas dinámicas
// (department/title/location), sin tocar email/rol/estado (como SetDisplayName con name).
// "" ⇒ NULL. Devuelve el usuario actualizado. El disparo de la reevaluación lo hace el
// handler en la misma tx (ReevaluateUserDynamicGroups).
func (s *Store) SetUserAttributes(ctx context.Context, q pg.Querier, userID, department, title, location string) (domain.User, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE users SET department = nullif($2,''), title = nullif($3,''), location = nullif($4,'')
		WHERE id = $1 RETURNING ` + userCols
	u, err := scanUser(q.QueryRow(ctx, sql, userID,
		domain.SanitizeDisplayName(department), domain.SanitizeDisplayName(title),
		domain.SanitizeDisplayName(location)))
	return u, errf("SetUserAttributes", err)
}
