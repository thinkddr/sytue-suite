// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// PIM (Privileged Identity Management, mig 0020): elegibilidades y activaciones JIT.
// Mismo aislamiento por tenant que roles/grupos (mig 0019): cada método recibe orgID y
// lo cruza con org_id en CADA query — un id de otro tenant no liga (ErrNotFound / 0
// filas), nunca una acción cross-tenant. El rol es role_id (personalizado) XOR
// builtin_role (texto); se proyecta role_name = coalesce(nombre del rol, builtin_role)
// para la UI. El «ahora» que fija/filtra ventanas llega SIEMPRE por parámetro (nunca
// SQL now()), para que la lógica de perms sea determinista y testeable.

// ---- Elegibilidades ----

// Eligibility es una fila del listado de elegibilidades, con datos de UI resueltos.
type Eligibility struct {
	ID                 string
	UserID             string
	UserEmail          string
	UserName           string
	RoleID             string // "" si built-in
	BuiltinRole        string // "" si personalizado
	RoleName           string // nombre del rol personalizado, o el built-in
	RequiresApproval   bool
	MaxDurationMinutes int
}

const eligSelect = `SELECT e.id, e.user_id, coalesce(u.email,''), coalesce(u.name,''),
	coalesce(e.role_id::text,''), coalesce(e.builtin_role,''),
	coalesce(cr.name, e.builtin_role, ''), e.requires_approval, e.max_duration_minutes
	FROM role_eligibilities e
	JOIN users u ON u.id = e.user_id
	LEFT JOIN custom_roles cr ON cr.id = e.role_id`

func scanEligibility(row pgx.Row) (Eligibility, error) {
	var e Eligibility
	err := row.Scan(&e.ID, &e.UserID, &e.UserEmail, &e.UserName, &e.RoleID,
		&e.BuiltinRole, &e.RoleName, &e.RequiresApproval, &e.MaxDurationMinutes)
	return e, err
}

func (s *Store) queryEligibilities(ctx context.Context, sql string, args ...any) ([]Eligibility, error) {
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, errf("queryEligibilities", err)
	}
	defer rows.Close()
	out := make([]Eligibility, 0, 16)
	for rows.Next() {
		e, err := scanEligibility(rows)
		if err != nil {
			return nil, errf("queryEligibilities.scan", err)
		}
		out = append(out, e)
	}
	return out, errf("queryEligibilities.iter", rows.Err())
}

// ListEligibilities lista las elegibilidades de la org (vista admin), orden estable por
// (email, rol).
func (s *Store) ListEligibilities(ctx context.Context, orgID string) ([]Eligibility, error) {
	return s.queryEligibilities(ctx, eligSelect+
		` WHERE e.org_id = $1 ORDER BY u.email, coalesce(cr.name, e.builtin_role)`, orgID)
}

// ListEligibilitiesForUser lista las elegibilidades de UN usuario en la org (vista self:
// «para qué roles puedo elevar»).
func (s *Store) ListEligibilitiesForUser(ctx context.Context, orgID, userID string) ([]Eligibility, error) {
	return s.queryEligibilities(ctx, eligSelect+
		` WHERE e.org_id = $1 AND e.user_id = $2 ORDER BY coalesce(cr.name, e.builtin_role)`, orgID, userID)
}

// GetEligibilityForUser resuelve la elegibilidad de un usuario para un rol concreto
// (role_id XOR builtin_role; exactamente uno no vacío). Es la verja de la activación
// self-service: sin fila ⇒ ErrNotFound ⇒ el usuario NO es elegible (403/404, no puede
// activar un rol para el que no se le declaró elegible). Devuelve requires_approval y
// max_duration para gobernar la activación.
func (s *Store) GetEligibilityForUser(ctx context.Context, orgID, userID, roleID, builtinRole string) (Eligibility, error) {
	sql := eligSelect + ` WHERE e.org_id = $1 AND e.user_id = $2 AND `
	var arg string
	if roleID != "" {
		sql += `e.role_id = $3::uuid`
		arg = roleID
	} else {
		sql += `e.builtin_role = $3`
		arg = builtinRole
	}
	e, err := scanEligibility(s.DB().QueryRow(ctx, sql, orgID, userID, arg))
	return e, errf("GetEligibilityForUser", err)
}

// CreateEligibility declara a un usuario ELEGIBLE para un rol en la org. role_id XOR
// builtin_role (exactamente uno no vacío; el CHECK de la tabla lo refuerza). Que el rol
// personalizado y el usuario pertenezcan a la org lo valida el handler ANTES (reusa
// GetCustomRole/GetMembership). Duplicado ⇒ índice único parcial 23505 ⇒ ErrConflict.
func (s *Store) CreateEligibility(ctx context.Context, orgID, userID, roleID, builtinRole string, requiresApproval bool, maxDuration int) (Eligibility, error) {
	const sql = `INSERT INTO role_eligibilities
		(org_id, user_id, role_id, builtin_role, requires_approval, max_duration_minutes)
		VALUES ($1, $2, nullif($3,'')::uuid, nullif($4,''), $5, $6) RETURNING id`
	var id string
	if err := s.DB().QueryRow(ctx, sql, orgID, userID, roleID, builtinRole, requiresApproval, maxDuration).Scan(&id); err != nil {
		return Eligibility{}, errf("CreateEligibility", err)
	}
	e, err := scanEligibility(s.DB().QueryRow(ctx, eligSelect+` WHERE e.id = $1`, id))
	return e, errf("CreateEligibility.read", err)
}

// DeleteEligibility borra una elegibilidad de la org (acotada por org_id). No afecta a
// activaciones ya en curso. Devuelve true si borró (existía en la org).
func (s *Store) DeleteEligibility(ctx context.Context, orgID, id string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM role_eligibilities WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return false, errf("DeleteEligibility", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ---- Activaciones ----

// Activation es una fila del listado de activaciones, con datos de UI resueltos. Las
// marcas activated_at/expires_at son nil mientras la activación está pending.
type Activation struct {
	ID              string
	UserID          string
	UserEmail       string
	UserName        string
	RoleID          string
	BuiltinRole     string
	RoleName        string
	Status          string
	Justification   string
	DurationMinutes int
	RequestedAt     time.Time
	ActivatedAt     *time.Time
	ExpiresAt       *time.Time
	ApprovedBy      string
	ApproverEmail   string
	// Perms son los permisos que concedería esta activación si está viva: los
	// del rol personalizado, o vacío cuando eleva a un rol integrado (ese
	// conjunto lo pone el dominio). Resueltos en el MISMO SELECT: el alcance
	// efectivo los necesita y pedirlos rol a rol sería un N+1 por activación.
	Perms []string
}

const actSelect = `SELECT ra.id, ra.user_id, coalesce(u.email,''), coalesce(u.name,''),
	coalesce(ra.role_id::text,''), coalesce(ra.builtin_role,''),
	coalesce(cr.name, ra.builtin_role, ''), ra.status, ra.justification,
	ra.duration_minutes, ra.requested_at, ra.activated_at, ra.expires_at,
	coalesce(ra.approved_by::text,''), coalesce(ap.email,''),
	coalesce(cr.permissions, '{}')
	FROM role_activations ra
	JOIN users u ON u.id = ra.user_id
	LEFT JOIN custom_roles cr ON cr.id = ra.role_id
	LEFT JOIN users ap ON ap.id = ra.approved_by`

func scanActivation(row pgx.Row) (Activation, error) {
	var a Activation
	err := row.Scan(&a.ID, &a.UserID, &a.UserEmail, &a.UserName, &a.RoleID, &a.BuiltinRole,
		&a.RoleName, &a.Status, &a.Justification, &a.DurationMinutes, &a.RequestedAt,
		&a.ActivatedAt, &a.ExpiresAt, &a.ApprovedBy, &a.ApproverEmail, &a.Perms)
	return a, err
}

// ListActivations lista activaciones de la org, filtrando opcionalmente por usuario
// (userID != "" ⇒ vista self) y/o estado (status != "" ⇒ vista admin de pendientes o
// vigentes). Orden por solicitud descendente, tope 200 (un tenant no tiene cientos de
// miles de activaciones VIGENTES; el histórico se acota por status).
func (s *Store) ListActivations(ctx context.Context, orgID, userID, status string) ([]Activation, error) {
	sql := actSelect + ` WHERE ra.org_id = $1`
	args := []any{orgID}
	if userID != "" {
		args = append(args, userID)
		sql += ` AND ra.user_id = $` + itoa(len(args))
	}
	if status != "" {
		args = append(args, status)
		sql += ` AND ra.status = $` + itoa(len(args))
	}
	sql += ` ORDER BY ra.requested_at DESC LIMIT 200`
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, errf("ListActivations", err)
	}
	defer rows.Close()
	out := make([]Activation, 0, 16)
	for rows.Next() {
		a, err := scanActivation(rows)
		if err != nil {
			return nil, errf("ListActivations.scan", err)
		}
		out = append(out, a)
	}
	return out, errf("ListActivations.iter", rows.Err())
}

// CreateActivation registra una activación. Si requiresApproval, queda 'pending' sin
// ventana (activated_at/expires_at NULL) hasta que un aprobador la conceda; si no, queda
// 'active' con ventana [now, now+duration] calculada con el `now` INYECTADO (no SQL
// now()). role_id XOR builtin_role. Un índice/uso concurrente no impone unicidad: dos
// activaciones activas del mismo rol solo re-conceden los mismos perms (unión idempotente
// en EffectivePerms). ponytail: sin unique-parcial por status 'active' — chocaría con la
// expiración perezosa (una fila caducada sigue en 'active' y bloquearía re-activar).
func (s *Store) CreateActivation(ctx context.Context, orgID, userID, roleID, builtinRole, justification string, duration int, requiresApproval bool, now time.Time) (Activation, error) {
	status := domain.ActivationActive
	var activatedAt, expiresAt *time.Time
	if requiresApproval {
		status = domain.ActivationPending
	} else {
		exp := now.Add(time.Duration(duration) * time.Minute)
		activatedAt, expiresAt = &now, &exp
	}
	const sql = `INSERT INTO role_activations
		(org_id, user_id, role_id, builtin_role, status, justification, duration_minutes, activated_at, expires_at)
		VALUES ($1, $2, nullif($3,'')::uuid, nullif($4,''), $5, $6, $7, $8, $9) RETURNING id`
	var id string
	if err := s.DB().QueryRow(ctx, sql, orgID, userID, roleID, builtinRole, status,
		justification, duration, activatedAt, expiresAt).Scan(&id); err != nil {
		return Activation{}, errf("CreateActivation", err)
	}
	a, err := scanActivation(s.DB().QueryRow(ctx, actSelect+` WHERE ra.id = $1`, id))
	return a, errf("CreateActivation.read", err)
}

// ApproveActivation concede una activación PENDIENTE: pending → active, fija el aprobador
// y la ventana [now, now + duration] (la duración guardada en la fila; el `now` llega
// inyectado). Acotada por org_id y a status='pending' (idempotente/anti doble-aprobación:
// 0 filas ⇒ ErrNotFound). Devuelve la fila resultante para auditar la ventana concedida.
func (s *Store) ApproveActivation(ctx context.Context, orgID, id, approverID string, now time.Time) (Activation, error) {
	// $4::timestamptz explícito: se usa como activated_at Y en aritmética de intervalo;
	// sin el cast, Postgres no deduce un tipo consistente para el parámetro (42P08).
	const sql = `UPDATE role_activations
		SET status = 'active', approved_by = $3, activated_at = $4::timestamptz,
		    expires_at = $4::timestamptz + (duration_minutes * interval '1 minute')
		WHERE id = $1 AND org_id = $2 AND status = 'pending'
		RETURNING id`
	var got string
	err := s.DB().QueryRow(ctx, sql, id, orgID, approverID, now).Scan(&got)
	if err != nil {
		return Activation{}, errf("ApproveActivation", err) // ErrNoRows ⇒ ErrNotFound
	}
	a, err := scanActivation(s.DB().QueryRow(ctx, actSelect+` WHERE ra.id = $1`, got))
	return a, errf("ApproveActivation.read", err)
}

// DenyActivation rechaza una activación PENDIENTE: pending → denied, registrando quién.
// Acotada por org_id y status='pending'. Devuelve true si transicionó (existía pendiente).
func (s *Store) DenyActivation(ctx context.Context, orgID, id, approverID string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `UPDATE role_activations SET status = 'denied', approved_by = $3
		WHERE id = $1 AND org_id = $2 AND status = 'pending'`, id, orgID, approverID)
	if err != nil {
		return false, errf("DenyActivation", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DeactivateActivation revoca una activación ACTIVA del PROPIO usuario (self-service):
// active → revoked. Acotada por org_id Y user_id (uno solo desactiva las suyas) y a
// status='active'. Los perms desaparecen en la próxima emisión de token. Devuelve true
// si transicionó.
func (s *Store) DeactivateActivation(ctx context.Context, orgID, userID, id string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `UPDATE role_activations SET status = 'revoked'
		WHERE id = $1 AND org_id = $2 AND user_id = $3 AND status = 'active'`, id, orgID, userID)
	if err != nil {
		return false, errf("DeactivateActivation", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ActivationPermsForUser devuelve las activaciones ACTIVE y VIGENTES (expires_at > now)
// del usuario en la org, como unidades para domain.EffectivePerms — la fuente PIM del
// claim `perms` en CADA emisión de token. El `now` llega inyectado y filtra la ventana en
// SQL (no SQL now()): acota el resultado a lo realmente vigente sin depender de un barrido,
// y mantiene la ruta del token barata aunque el histórico crezca. Sin activaciones ⇒ nil.
func (s *Store) ActivationPermsForUser(ctx context.Context, orgID, userID string, now time.Time) ([]domain.PIMActivation, error) {
	const sql = `SELECT coalesce(cr.permissions, '{}'), (ra.role_id IS NULL), ra.expires_at
		FROM role_activations ra
		LEFT JOIN custom_roles cr ON cr.id = ra.role_id
		WHERE ra.org_id = $1 AND ra.user_id = $2 AND ra.status = 'active' AND ra.expires_at > $3`
	rows, err := s.DB().Query(ctx, sql, orgID, userID, now)
	if err != nil {
		return nil, errf("ActivationPermsForUser", err)
	}
	defer rows.Close()
	var out []domain.PIMActivation
	for rows.Next() {
		a := domain.PIMActivation{Status: domain.ActivationActive}
		if err := rows.Scan(&a.Perms, &a.Builtin, &a.ExpiresAt); err != nil {
			return nil, errf("ActivationPermsForUser.scan", err)
		}
		out = append(out, a)
	}
	return out, errf("ActivationPermsForUser.iter", rows.Err())
}
