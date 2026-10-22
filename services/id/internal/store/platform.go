// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Consultas de ADMINISTRACIÓN DE PLATAFORMA (mig 0031). Son las únicas del store
// que atraviesan organizaciones a propósito: soporte y dirección necesitan ver la
// plataforma entera. Quién puede llamarlas lo decide `platform_admins`, y eso se
// comprueba en el middleware HTTP, no aquí.
//
// Ninguna de estas consultas se usa desde las APIs de organización.

// PlatformOrg es una organización vista desde plataforma: además de sus datos,
// el aforo (vendido + regalado) y el consumo real de miembros activos.
type PlatformOrg struct {
	domain.Organization
	// MergedInto es el destino si la organización fue absorbida ('' si no).
	MergedInto     string
	MergedAt       *time.Time
	ActiveUsers    int
	SeatsPurchased int
	SeatsGranted   int
	SubStatus      string
	OwnerEmails    []string
}

// SeatsEffective es el aforo real: lo comprado más lo regalado vigente. Es el
// número contra el que hay que comparar los usuarios activos, no `seats`.
func (o PlatformOrg) SeatsEffective() int { return o.SeatsPurchased + o.SeatsGranted }

// listOrgsSQL calcula aforo y consumo en la misma consulta. Las subconsultas van
// como LATERAL agregadas por organización: con un JOIN plano, una org con tres
// concesiones multiplicaría su recuento de miembros por tres.
const listOrgsSQL = `
	SELECT o.id, o.slug, o.name, o.status, o.created_at, o.updated_at,
	       coalesce(o.merged_into_org_id::text, ''), o.merged_at,
	       coalesce(m.activos, 0),
	       coalesce(s.seats, 0), coalesce(s.status, ''),
	       coalesce(g.regaladas, 0),
	       coalesce(ow.correos, ARRAY[]::text[]),
	       coalesce(o.education, false)
	FROM organizations o
	LEFT JOIN LATERAL (
	    SELECT count(*) AS activos
	    FROM memberships ms
	    JOIN users u ON u.id = ms.user_id AND u.status = 'active'
	    WHERE ms.org_id = o.id
	) m ON true
	LEFT JOIN org_subscriptions s ON s.org_id = o.id
	LEFT JOIN LATERAL (
	    SELECT sum(sg.seats)::int AS regaladas
	    FROM seat_grants sg
	    WHERE sg.org_id = o.id
	      AND sg.revoked_at IS NULL
	      AND (sg.expires_at IS NULL OR sg.expires_at > now())
	) g ON true
	LEFT JOIN LATERAL (
	    SELECT array_agg(u.email ORDER BY u.email) AS correos
	    FROM memberships ms
	    JOIN users u ON u.id = ms.user_id
	    WHERE ms.org_id = o.id AND ms.role = 'owner'
	) ow ON true`

// scanPlatformOrgs NO cierra las filas: lo hace el llamador con defer, que es
// donde el análisis estático puede comprobarlo.
func scanPlatformOrgs(rows pgx.Rows) ([]PlatformOrg, error) {
	var out []PlatformOrg
	for rows.Next() {
		var o PlatformOrg
		if err := rows.Scan(&o.ID, &o.Slug, &o.Name, &o.Status, &o.CreatedAt, &o.UpdatedAt,
			&o.MergedInto, &o.MergedAt, &o.ActiveUsers,
			&o.SeatsPurchased, &o.SubStatus, &o.SeatsGranted, &o.OwnerEmails, &o.Education); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ListAllOrgs devuelve TODAS las organizaciones de la plataforma, más recientes
// primero. Sin paginación por keyset como en la Console: el operador de
// plataforma necesita buscar y ordenar sobre el conjunto, y el número de
// organizaciones es de otro orden de magnitud que el de usuarios. `limit` acota
// la respuesta igualmente para que un incidente no devuelva un JSON de 50 MB.
func (s *Store) ListAllOrgs(ctx context.Context, q string, limit int) ([]PlatformOrg, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	const sql = listOrgsSQL + `
		WHERE $1 = '' OR plegar(o.slug) LIKE '%' || plegar($1) || '%' OR plegar(o.name) LIKE '%' || plegar($1) || '%'
		ORDER BY o.created_at DESC, o.id DESC
		LIMIT $2`
	rows, err := s.DB().Query(ctx, sql, q, limit)
	if err != nil {
		return nil, errf("ListAllOrgs", err)
	}
	defer rows.Close()
	out, err := scanPlatformOrgs(rows)
	return out, errf("ListAllOrgs", err)
}

// GetPlatformOrg devuelve una organización con su aforo y consumo.
func (s *Store) GetPlatformOrg(ctx context.Context, orgID string) (PlatformOrg, error) {
	const sql = listOrgsSQL + ` WHERE o.id = $1`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return PlatformOrg{}, errf("GetPlatformOrg", err)
	}
	defer rows.Close()
	out, err := scanPlatformOrgs(rows)
	if err != nil {
		return PlatformOrg{}, errf("GetPlatformOrg", err)
	}
	if len(out) == 0 {
		return PlatformOrg{}, domain.ErrNotFound
	}
	return out[0], nil
}

// PlatformUsage es la foto agregada de la plataforma.
type PlatformUsage struct {
	Orgs           int `json:"orgs"`
	OrgsActive     int `json:"orgs_active"`
	OrgsSuspended  int `json:"orgs_suspended"`
	OrgsMerged     int `json:"orgs_merged"`
	Users          int `json:"users"`
	UsersActive    int `json:"users_active"`
	UsersLast30d   int `json:"users_active_30d"`
	SeatsPurchased int `json:"seats_purchased"`
	SeatsGranted   int `json:"seats_granted"`
	SubsActive     int `json:"subscriptions_active"`
}

// PlatformUsageNow calcula el resumen de la plataforma. «Activo en 30 días» se
// mide por sesión creada, no por login: una sesión viva es la evidencia de uso
// que tenemos sin instrumentar cada servicio, y no se pierde al renovar tokens.
func (s *Store) PlatformUsageNow(ctx context.Context, now time.Time) (PlatformUsage, error) {
	const sql = `
		SELECT
		  (SELECT count(*) FROM organizations),
		  (SELECT count(*) FROM organizations WHERE status = 'active'),
		  (SELECT count(*) FROM organizations WHERE status = 'suspended'),
		  (SELECT count(*) FROM organizations WHERE status = 'merged'),
		  (SELECT count(*) FROM users),
		  (SELECT count(*) FROM users WHERE status = 'active'),
		  (SELECT count(DISTINCT user_id) FROM sessions WHERE created_at > $1),
		  (SELECT coalesce(sum(seats), 0)::int FROM org_subscriptions WHERE status = 'active'),
		  (SELECT coalesce(sum(seats), 0)::int FROM seat_grants
		     WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())),
		  (SELECT count(*) FROM org_subscriptions WHERE status = 'active')`
	var u PlatformUsage
	err := s.DB().QueryRow(ctx, sql, now.Add(-30*24*time.Hour)).Scan(
		&u.Orgs, &u.OrgsActive, &u.OrgsSuspended, &u.OrgsMerged,
		&u.Users, &u.UsersActive, &u.UsersLast30d,
		&u.SeatsPurchased, &u.SeatsGranted, &u.SubsActive,
	)
	return u, errf("PlatformUsageNow", err)
}

// SetOrgStatus cambia el estado de una organización (suspender/reactivar) sin
// tocar su nombre. Existe aparte de UpdateOrgSettings porque plataforma no debe
// necesitar reenviar el nombre para suspender, y porque 'merged' no se alcanza
// nunca por esta vía (lo fija la fusión, que además fija el destino).
func (s *Store) SetOrgStatus(ctx context.Context, q pg.Querier, orgID string, st domain.Status) (domain.Organization, error) {
	if s.needsExternalAccessTransaction(q) {
		var out domain.Organization
		err := s.InTx(ctx, func(tx pg.Querier) (err error) {
			out, err = s.SetOrgStatus(ctx, tx, orgID, st)
			return err
		})
		return out, err
	}
	if err := s.suspendExternalOrg(ctx, q, orgID, st); err != nil {
		return domain.Organization{}, err
	}
	// Una organización FUSIONADA sí se puede ELIMINAR, y antes no: el `status <>
	// 'merged'` de aquí lo impedía para cualquier estado. Tenía sentido para
	// suspender o reactivar —una organización cuyos datos ya están en otra no se
	// «vuelve a activar»—, pero no para retirarla de la lista: fusionar deja la
	// fila para siempre y la consola no ofrecía forma de limpiarla.
	//
	// Reactivar una fusionada sigue prohibido, y eso lo comprueba el handler: es
	// la operación que de verdad rompería (dos organizaciones sirviendo los mismos
	// datos migrados).
	const sql = `
		UPDATE organizations SET status = $2, updated_at = now()
		WHERE id = $1 AND (status <> 'merged' OR $2 = 'deleted')
		RETURNING id, slug, name, status, created_at, updated_at`
	var o domain.Organization
	err := q.QueryRow(ctx, sql, orgID, string(st)).
		Scan(&o.ID, &o.Slug, &o.Name, &o.Status, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return domain.Organization{}, errf("SetOrgStatus", err)
	}
	if st == domain.StatusActive {
		if err := s.queueExternalOrgActivation(ctx, q, orgID); err != nil {
			return domain.Organization{}, err
		}
	}
	return o, nil
}

// --- Plazas concedidas ------------------------------------------------------

// SeatGrant es una concesión de plazas gratuitas (gesto comercial, empleado…).
type SeatGrant struct {
	ID        string     `json:"id"`
	OrgID     string     `json:"org_id"`
	Seats     int        `json:"seats"`
	Reason    string     `json:"reason"`
	Kind      string     `json:"kind"`
	GrantedBy string     `json:"granted_by"`
	ExpiresAt *time.Time `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// SeatGrantKinds son los tipos admitidos; deben coincidir con el CHECK de la
// migración. Se valida en Go además de en la base para poder responder 400 con
// un mensaje útil en vez de un error de restricción.
var SeatGrantKinds = []string{"goodwill", "employee", "trial", "migration"}

// CreateSeatGrant concede plazas gratuitas a una organización.
func (s *Store) CreateSeatGrant(ctx context.Context, q pg.Querier, g SeatGrant) (SeatGrant, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `
		INSERT INTO seat_grants (org_id, seats, reason, kind, granted_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, org_id, seats, reason, kind, granted_by, expires_at, revoked_at, created_at`
	var out SeatGrant
	err := q.QueryRow(ctx, sql, g.OrgID, g.Seats, g.Reason, g.Kind, g.GrantedBy, g.ExpiresAt).
		Scan(&out.ID, &out.OrgID, &out.Seats, &out.Reason, &out.Kind, &out.GrantedBy,
			&out.ExpiresAt, &out.RevokedAt, &out.CreatedAt)
	return out, errf("CreateSeatGrant", err)
}

// ListSeatGrants devuelve las concesiones de una organización, vigentes y
// pasadas: el historial ES la justificación de la factura.
func (s *Store) ListSeatGrants(ctx context.Context, orgID string) ([]SeatGrant, error) {
	const sql = `
		SELECT id, org_id, seats, reason, kind, granted_by, expires_at, revoked_at, created_at
		FROM seat_grants WHERE org_id = $1 ORDER BY created_at DESC`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("ListSeatGrants", err)
	}
	defer rows.Close()
	var out []SeatGrant
	for rows.Next() {
		var g SeatGrant
		if err := rows.Scan(&g.ID, &g.OrgID, &g.Seats, &g.Reason, &g.Kind, &g.GrantedBy,
			&g.ExpiresAt, &g.RevokedAt, &g.CreatedAt); err != nil {
			return nil, errf("ListSeatGrants", err)
		}
		out = append(out, g)
	}
	return out, errf("ListSeatGrants", rows.Err())
}

// RevokeSeatGrant retira una concesión. No la borra: el registro de que se
// regalaron plazas durante un periodo es lo que explica una factura antigua.
//
// Devuelve la concesión retirada. La necesita quien audita: sin ella, el registro
// de plataforma solo puede escribir el uuid de la concesión, y una fila que dice
// «se retiraron unas plazas» sin decir a quién ni cuántas no responde la pregunta
// que se hace después. Un uuid tampoco se puede resolver luego: la fila sigue
// ahí, pero nadie va a ir a buscarla.
func (s *Store) RevokeSeatGrant(ctx context.Context, q pg.Querier, grantID string) (SeatGrant, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE seat_grants SET revoked_at = now(), updated_at = now()
	             WHERE id = $1 AND revoked_at IS NULL
	             RETURNING id, org_id, seats, reason, kind, granted_by, expires_at, revoked_at, created_at`
	var g SeatGrant
	err := q.QueryRow(ctx, sql, grantID).Scan(&g.ID, &g.OrgID, &g.Seats, &g.Reason,
		&g.Kind, &g.GrantedBy, &g.ExpiresAt, &g.RevokedAt, &g.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// O no existe, o ya estaba retirada. Para quien llama es lo mismo.
			return SeatGrant{}, domain.ErrNotFound
		}
		return SeatGrant{}, errf("RevokeSeatGrant", err)
	}
	return g, nil
}

// --- Admins de plataforma ---------------------------------------------------

// PlatformAdmin es una inscripción en la allowlist global.
type PlatformAdmin struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	GrantedBy string    `json:"granted_by"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
	// Última revisión del acceso (mig 0047). Nunca es cero: conceder el alcance
	// cuenta como haberlo mirado una vez, y el alta rellena esta fecha.
	ReviewedAt time.Time `json:"reviewed_at"`
	ReviewedBy string    `json:"reviewed_by"`
}

// ListPlatformAdmins devuelve quién tiene alcance global, con su correo.
func (s *Store) ListPlatformAdmins(ctx context.Context) ([]PlatformAdmin, error) {
	const sql = `
		SELECT pa.user_id, u.email, u.name, coalesce(pa.granted_by::text, ''), pa.reason,
			pa.created_at, pa.reviewed_at, coalesce(pa.reviewed_by::text, '')
		FROM platform_admins pa JOIN users u ON u.id = pa.user_id
		ORDER BY pa.reviewed_at`
	rows, err := s.DB().Query(ctx, sql)
	if err != nil {
		return nil, errf("ListPlatformAdmins", err)
	}
	defer rows.Close()
	var out []PlatformAdmin
	for rows.Next() {
		var a PlatformAdmin
		if err := rows.Scan(&a.UserID, &a.Email, &a.Name, &a.GrantedBy, &a.Reason,
			&a.CreatedAt, &a.ReviewedAt, &a.ReviewedBy); err != nil {
			return nil, errf("ListPlatformAdmins", err)
		}
		out = append(out, a)
	}
	return out, errf("ListPlatformAdmins", rows.Err())
}

// ReviewPlatformAdmin sella una revisión: alguien ha mirado este acceso hoy y
// dice que sigue haciendo falta. Devuelve false si ese usuario ya no lo tiene.
//
// Revisar el PROPIO acceso no se impide aquí sino en el handler: es una decisión
// de política, y esta capa solo escribe la fecha.
func (s *Store) ReviewPlatformAdmin(ctx context.Context, q pg.Querier, userID, reviewedBy string, now time.Time) (bool, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `UPDATE platform_admins
		SET reviewed_at = $2, reviewed_by = $3 WHERE user_id = $1`, userID, now, reviewedBy)
	if err != nil {
		return false, errf("ReviewPlatformAdmin", err)
	}
	return tag.RowsAffected() > 0, nil
}

// CountPlatformAdmins existe para impedir que la plataforma se quede sin ningún
// admin: revocar el último dejaría la administración global inaccesible salvo
// volviendo a ejecutar el bootstrap del despliegue.
func (s *Store) CountPlatformAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.DB().QueryRow(ctx, `SELECT count(*) FROM platform_admins`).Scan(&n)
	return n, errf("CountPlatformAdmins", err)
}

// RevokePlatformAdmin retira el alcance global de un usuario.
func (s *Store) RevokePlatformAdmin(ctx context.Context, q pg.Querier, userID string) error {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `DELETE FROM platform_admins WHERE user_id = $1`, userID)
	if err != nil {
		return errf("RevokePlatformAdmin", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
