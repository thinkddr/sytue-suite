// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/commerce"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func itoa(n int) string { return strconv.Itoa(n) }

const orgCols = `id, slug, name, status, kind, education, color, forma, created_at, updated_at`

func scanOrg(row pgx.Row) (domain.Organization, error) {
	var o domain.Organization
	err := row.Scan(&o.ID, &o.Slug, &o.Name, &o.Status, &o.Kind, &o.Education, &o.Color, &o.Forma, &o.CreatedAt, &o.UpdatedAt)
	return o, err
}

// CreateOrg inserta una organización (tenant).
func (s *Store) CreateOrg(ctx context.Context, q pg.Querier, o domain.Organization) (domain.Organization, error) {
	if q == nil {
		var got domain.Organization
		err := s.InTx(ctx, func(tx pg.Querier) error {
			var err error
			got, err = s.CreateOrg(ctx, tx, o)
			return err
		})
		return got, err
	}
	kind := o.Kind
	if kind == "" {
		kind = domain.OrgKindOrg
	}
	const sql = `INSERT INTO organizations (slug, name, status, kind)
		VALUES ($1,$2,$3,$4) RETURNING ` + orgCols
	got, err := scanOrg(q.QueryRow(ctx, sql, o.Slug, o.Name, defaultStatus(o.Status), kind))
	if err != nil {
		return got, errf("CreateOrg", err)
	}
	// Una cuenta personal nace ya con su contrato gratuito; una organización,
	// con la suite pendiente de pago, como siempre.
	plan := commerce.PlanPro
	if kind == domain.OrgKindPersonal {
		plan = commerce.PlanPersonalFree
	}
	if err := s.EnsureSubscriptionPlan(ctx, q, got.ID, 1, plan); err != nil {
		return domain.Organization{}, err
	}
	return got, nil
}

// GetOrgBySlug recupera una organización por slug.
func (s *Store) GetOrgBySlug(ctx context.Context, slug string) (domain.Organization, error) {
	const sql = `SELECT ` + orgCols + ` FROM organizations WHERE slug = $1`
	o, err := scanOrg(s.DB().QueryRow(ctx, sql, slug))
	return o, errf("GetOrgBySlug", err)
}

// GetOrgByID recupera una organización por id.
func (s *Store) GetOrgByID(ctx context.Context, id string) (domain.Organization, error) {
	const sql = `SELECT ` + orgCols + ` FROM organizations WHERE id = $1`
	o, err := scanOrg(s.DB().QueryRow(ctx, sql, id))
	return o, errf("GetOrgByID", err)
}

// EstadoDeOrgs devuelve el estado de ciclo de vida de cada slug conocido:
// "active", "suspended", "merged" o "deleted". Los slugs desconocidos no
// aparecen en el mapa; el llamante los trata como "deleted".
// Hasta 500 slugs en una sola consulta (§6.5 del contrato de Pages).
func (s *Store) EstadoDeOrgs(ctx context.Context, slugs []string) (map[string]string, error) {
	out := make(map[string]string, len(slugs))
	if len(slugs) == 0 {
		return out, nil
	}
	rows, err := s.DB().Query(ctx,
		`SELECT slug, status FROM organizations WHERE slug = ANY($1::text[])`,
		slugs)
	if err != nil {
		return nil, errf("EstadoDeOrgs", err)
	}
	defer rows.Close()
	for rows.Next() {
		var slug, status string
		if err := rows.Scan(&slug, &status); err != nil {
			return nil, errf("EstadoDeOrgs.scan", err)
		}
		out[slug] = status
	}
	return out, errf("EstadoDeOrgs.rows", rows.Err())
}

// NombresDeOrgs da el nombre visible de las organizaciones ACTIVAS de `slugs`. Una
// cuenta personal no sale: su «organización» es una persona, y su nombre, el suyo.
func (s *Store) NombresDeOrgs(ctx context.Context, slugs []string) (map[string]string, error) {
	out := make(map[string]string, len(slugs))
	if len(slugs) == 0 {
		return out, nil
	}
	rows, err := s.DB().Query(ctx,
		`SELECT slug, name FROM organizations WHERE slug = ANY($1::text[]) AND status = 'active' AND kind = 'org'`,
		slugs)
	if err != nil {
		return nil, errf("NombresDeOrgs", err)
	}
	defer rows.Close()
	for rows.Next() {
		var slug, nombre string
		if err := rows.Scan(&slug, &nombre); err != nil {
			return nil, errf("NombresDeOrgs.scan", err)
		}
		out[slug] = nombre
	}
	return out, errf("NombresDeOrgs.rows", rows.Err())
}

// UpdateOrgSettings actualiza los ajustes editables de una organización (nombre y
// estado de ciclo de vida). El slug es INMUTABLE por diseño: es la identidad del
// tenant de la que derivan namespace Kapsule, prefijo de almacenamiento y grupo
// `org/<slug>` en toda la suite — renombrarlo es una migración cross-servicio, no
// un ajuste. `deleted` no se alcanza por esta vía (baja lógica es otro flujo).
func (s *Store) UpdateOrgSettings(ctx context.Context, q pg.Querier, orgID, name string, status domain.Status) (domain.Organization, error) {
	if s.needsExternalAccessTransaction(q) {
		var out domain.Organization
		err := s.InTx(ctx, func(tx pg.Querier) (err error) {
			out, err = s.UpdateOrgSettings(ctx, tx, orgID, name, status)
			return err
		})
		return out, err
	}
	if err := s.suspendExternalOrg(ctx, q, orgID, defaultStatus(status)); err != nil {
		return domain.Organization{}, err
	}
	const sql = `UPDATE organizations SET name = $2, status = $3, updated_at = now()
		WHERE id = $1 AND status <> 'deleted' RETURNING ` + orgCols
	got, err := scanOrg(q.QueryRow(ctx, sql, orgID, domain.SanitizeDisplayName(name), defaultStatus(status)))
	if err == nil && got.Status == domain.StatusActive {
		err = s.queueExternalOrgActivation(ctx, q, orgID)
	}
	return got, errf("UpdateOrgSettings", err)
}

// GetOrgPasswordPolicy lee la política de contraseñas de una org.
func (s *Store) GetOrgPasswordPolicy(ctx context.Context, orgID string) (domain.PasswordPolicy, error) {
	var p domain.PasswordPolicy
	err := s.DB().QueryRow(ctx,
		`SELECT password_min_length, password_require_complexity FROM organizations WHERE id=$1`, orgID).
		Scan(&p.MinLength, &p.RequireComplexity)
	return p, errf("GetOrgPasswordPolicy", err)
}

// UpdateOrgPasswordPolicy fija la política de contraseñas de una org.
func (s *Store) UpdateOrgPasswordPolicy(ctx context.Context, q pg.Querier, orgID string, p domain.PasswordPolicy) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx,
		`UPDATE organizations SET password_min_length=$2, password_require_complexity=$3, updated_at=now()
		 WHERE id=$1 AND status <> 'deleted'`, orgID, p.MinLength, p.RequireComplexity)
	return errf("UpdateOrgPasswordPolicy", err)
}

// EffectivePasswordPolicyForUser devuelve la política que se aplica al fijar la
// contraseña de un usuario: la MÁS ESTRICTA entre las orgs de las que es miembro
// (mayor longitud mínima y complejidad si CUALQUIERA la exige). Un usuario en
// varias orgs no puede eludir la política dura de una eligiendo la blanda de otra.
// Sin ninguna membresía cae al default (mismo default que la migración), de una
// sola consulta agregada.
func (s *Store) EffectivePasswordPolicyForUser(ctx context.Context, userID string) (domain.PasswordPolicy, error) {
	d := domain.DefaultPasswordPolicy
	var p domain.PasswordPolicy
	err := s.DB().QueryRow(ctx, `
		SELECT coalesce(max(o.password_min_length), $2),
		       coalesce(bool_or(o.password_require_complexity), $3)
		FROM memberships m JOIN organizations o ON o.id = m.org_id
		WHERE m.user_id = $1`, userID, d.MinLength, d.RequireComplexity).
		Scan(&p.MinLength, &p.RequireComplexity)
	if err != nil {
		return d, errf("EffectivePasswordPolicyForUser", err)
	}
	return p, nil
}

// ManagedOrg es una organización que un administrador puede gestionar, con el
// recuento de miembros para el listado de Console.
type ManagedOrg struct {
	ID        string
	Slug      string
	Name      string
	Status    domain.Status
	UserCount int
	CreatedAt time.Time
	// Role es el rol del SOLICITANTE en esta organización (owner|admin), no el de
	// nadie más. Va en el listado porque el claim `roles` del token solo habla de
	// la org activa: quien gestiona dos orgs no puede saber por el token si en la
	// otra es owner o admin, y esa diferencia decide qué ofrece la interfaz (solo
	// un owner puede conceder el rol owner).
	Role domain.Role
}

// OrgCursor es la posición keyset (created_at, id) de la última org devuelta por
// ListManagedOrgsPage. nil ⇒ primera página.
type OrgCursor struct {
	CreatedAt time.Time
	ID        string
}

// ListManagedOrgsPage lista las organizaciones que un usuario puede GESTIONAR —
// aquellas en las que es owner o admin (fail-closed: un simple member no las ve) —
// ordenadas por (created_at, id) DESC con paginación keyset, igual que
// ListOrgUsersPage. Aísla por membresía: nunca devuelve una org de la que el
// usuario no es administrador, así que la lista ES el alcance de autorización del
// admin (no hay super-admin de plataforma en el modelo). Devuelve hasta limit
// filas y more=true si queda al menos una página más (consulta limit+1).
func (s *Store) ListManagedOrgsPage(ctx context.Context, userID string, after *OrgCursor, limit int) ([]ManagedOrg, bool, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// El JOIN por (user_id, role) usa memberships_user_idx; status<>'deleted' oculta
	// las orgs dadas de baja y kind='org' la cuenta personal de quien administra, que
	// es suya pero no se administra desde Sytue Admin (ADR 0038). user_count por
	// subconsulta correlacionada sobre memberships_pkey (org_id lidera la PK).
	// ponytail: user_count es O(miembros) por org; trivial a escala normal (decenas–
	// cientos de miembros). Si una org crece a cientos de miles, denormalizar un
	// contador member_count en organizations mantenido en cada alta/baja de membresía.
	const base = `SELECT o.id, o.slug, o.name, o.status, o.created_at,
		(SELECT count(*) FROM memberships mc WHERE mc.org_id = o.id) AS user_count,
		m.role
		FROM organizations o
		JOIN memberships m ON m.org_id = o.id AND m.user_id = $1 AND m.role IN ('owner','admin')
		WHERE o.status <> 'deleted' AND o.kind = 'org'`
	const tail = ` ORDER BY o.created_at DESC, o.id DESC LIMIT `
	var (
		sql  string
		args []any
	)
	if after == nil {
		sql = base + tail + `$2`
		args = []any{userID, limit + 1}
	} else {
		sql = base + ` AND (o.created_at, o.id) < ($2::timestamptz, $3::uuid)` + tail + `$4`
		args = []any{userID, after.CreatedAt, after.ID, limit + 1}
	}
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, false, errf("ListManagedOrgsPage", err)
	}
	defer rows.Close()
	out := make([]ManagedOrg, 0, limit)
	for rows.Next() {
		var o ManagedOrg
		if err := rows.Scan(&o.ID, &o.Slug, &o.Name, &o.Status, &o.CreatedAt, &o.UserCount, &o.Role); err != nil {
			return nil, false, errf("ListManagedOrgsPage.scan", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, false, errf("ListManagedOrgsPage.iter", err)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// UpsertMembership crea o actualiza el rol de un usuario en una organización.
func (s *Store) UpsertMembership(ctx context.Context, q pg.Querier, m domain.Membership) error {
	if s.needsExternalAccessTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error {
			return s.UpsertMembership(ctx, tx, m)
		})
	}
	// Si la org ya está pagando, no puede superar silenciosamente los asientos:
	// todas las vías (invitación, SCIM, JIT SSO y admin) convergen aquí. Las orgs
	// pending necesitan poder crear su primer owner antes del checkout. El lock
	// transaccional por organización evita que dos altas concurrentes consuman el
	// mismo último asiento.
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, m.OrgID); err != nil {
		return errf("UpsertMembership lock", err)
	}
	const sql = `INSERT INTO memberships (org_id, user_id, role)
		SELECT $1,$2,$3 WHERE
			exists (SELECT 1 FROM memberships WHERE org_id=$1 AND user_id=$2)
			OR NOT EXISTS (SELECT 1 FROM org_subscriptions WHERE org_id=$1 AND status='active')
			OR (SELECT count(*) FROM memberships m JOIN users u ON u.id=m.user_id
				WHERE m.org_id=$1 AND u.status='active') <
			   (SELECT seats FROM org_subscriptions WHERE org_id=$1)
		ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role RETURNING 1`
	var one int
	err := q.QueryRow(ctx, sql, m.OrgID, m.UserID, m.Role).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConflict
	}
	if err != nil {
		return errf("UpsertMembership", err)
	}
	// El buzón se asigna AQUÍ, en la misma transacción que la membresía, por lo
	// mismo que los asientos: es el punto por el que pasan las cuatro vías de alta
	// (invitación aceptada, SCIM, SSO con aprovisionamiento al vuelo y consola).
	// Colgarlo de cada una por separado habría sido cuatro sitios que mantener y,
	// en cuanto apareciera la quinta, un alta sin correo que nadie notaría hasta
	// que alguien intentara escribirle. En la misma transacción para que no exista
	// el estado intermedio «es miembro pero no tiene dirección».
	//
	// Idempotente: quien ya tiene buzón conserva el suyo, así que un cambio de rol
	// —que también entra por aquí— no reescribe la dirección de nadie. Es también
	// lo que deja personalizar la dirección sin propagar un parámetro hasta aquí:
	// quien da de alta reserva antes la dirección elegida (ver materializeInvite) y
	// esta llamada la encuentra puesta y no hace nada.
	if _, err := s.EnsureOrgMailbox(ctx, q, m.OrgID, m.UserID, ""); err != nil {
		return err
	}
	return s.queueExternalActivation(ctx, q, m.UserID, m.OrgID)
}

// RemoveMembership elimina la membresía de un usuario en una organización y
// purga sus pertenencias a grupos de ESA org. Los grupos son portadores de authz
// (claim `groups` del JWT ⇒ shares por-grupo de Drive) y group_members solo cae en
// cascada al borrar el grupo o el usuario, NO al quitar la membresía; sin esta
// purga quedarían filas huérfanas que reconcederían todo si al usuario se le
// re-invita. Acotado por org_id: no toca los grupos del usuario en otras orgs.
// Debe correr en la misma tx que el borrado de la membresía.
func (s *Store) RemoveMembership(ctx context.Context, q pg.Querier, orgID, userID string) error {
	if s.needsExternalAccessTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error { return s.RemoveMembership(ctx, tx, orgID, userID) })
	}
	if err := s.removeExternalOrgAccess(ctx, q, orgID, userID); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `DELETE FROM group_members gm USING groups g
		WHERE gm.group_id = g.id AND g.org_id = $1 AND gm.user_id = $2`, orgID, userID); err != nil {
		return errf("RemoveMembership.groups", err)
	}
	if _, err := q.Exec(ctx, `DELETE FROM memberships WHERE org_id=$1 AND user_id=$2`, orgID, userID); err != nil {
		return errf("RemoveMembership", err)
	}
	// Una reinvitación no revive las contraseñas de la membresía retirada.
	if _, err := q.Exec(ctx, `UPDATE app_passwords SET revoked_at = now()
		WHERE org_id = $1 AND user_id = $2 AND revoked_at IS NULL`, orgID, userID); err != nil {
		return errf("RemoveMembership.appPasswords", err)
	}
	// La baja ARRASTRA las credenciales de esa org: sin esto quedaba un refresh token
	// válido con su `org_id`, y grantRefresh seguía mintando access tokens con el claim
	// `tenant` de la organización (el rol salía vacío, pero el tenant NO, porque se
	// resuelve por GetOrgByID sin comprobar membresía). Toda la suite deriva el tenant
	// de ese claim, así que la baja no surtía efecto hasta que caducara el refresh.
	// grantRefresh revalida además la membresía: esta limpieza es la raíz, esa es la verja.
	return s.RevokeRefreshTokensForUserInOrg(ctx, q, userID, orgID)
}

// ListMembershipsForUser devuelve las membresías (org+rol) de un usuario.
func (s *Store) ListMembershipsForUser(ctx context.Context, userID string) ([]domain.Membership, error) {
	const sql = `SELECT org_id, user_id, role, created_at
		FROM memberships WHERE user_id = $1 ORDER BY created_at`
	rows, err := s.DB().Query(ctx, sql, userID)
	if err != nil {
		return nil, errf("ListMembershipsForUser", err)
	}
	defer rows.Close()
	var out []domain.Membership
	for rows.Next() {
		var m domain.Membership
		if err := rows.Scan(&m.OrgID, &m.UserID, &m.Role, &m.CreatedAt); err != nil {
			return nil, errf("ListMembershipsForUser.scan", err)
		}
		out = append(out, m)
	}
	return out, errf("ListMembershipsForUser.iter", rows.Err())
}

// OrgMembership es una membresía CON los datos de su organización. Existe para
// el selector de organización y para `GET /me`: con solo el `org_id` de
// domain.Membership no se puede pintar una lista donde elegir, y resolver el
// nombre de cada una con una consulta aparte es el N+1 de siempre.
type OrgMembership struct {
	OrgID  string
	Slug   string
	Name   string
	Role   domain.Role
	Status domain.Status
	Kind   domain.OrgKind
}

// ListOrgMembershipsForUser devuelve las organizaciones del usuario con su rol.
// Mismo orden que ListMembershipsForUser (por antigüedad de la membresía), que
// es el que decide cuál es la activa cuando el usuario no ha elegido ninguna.
func (s *Store) ListOrgMembershipsForUser(ctx context.Context, userID string) ([]OrgMembership, error) {
	const sql = `SELECT m.org_id, o.slug, o.name, m.role, o.status, o.kind
		FROM memberships m JOIN organizations o ON o.id = m.org_id
		WHERE m.user_id = $1 ORDER BY m.created_at`
	rows, err := s.DB().Query(ctx, sql, userID)
	if err != nil {
		return nil, errf("ListOrgMembershipsForUser", err)
	}
	defer rows.Close()
	var out []OrgMembership
	for rows.Next() {
		var m OrgMembership
		if err := rows.Scan(&m.OrgID, &m.Slug, &m.Name, &m.Role, &m.Status, &m.Kind); err != nil {
			return nil, errf("ListOrgMembershipsForUser.scan", err)
		}
		out = append(out, m)
	}
	return out, errf("ListOrgMembershipsForUser.iter", rows.Err())
}

// SCIMStatus son los números del aprovisionamiento automático de UNA
// organización: cuánta gente entró por SCIM y cuándo fue la última vez que el
// IdP corporativo tocó algo.
type SCIMStatus struct {
	Provisioned int
	LastEventAt *time.Time
}

// SCIMStatusForOrg mide el aprovisionamiento de una organización.
//
// Las dos cifras salen de datos que ya existen —`users.external_id`, que solo
// escribe SCIM, y los eventos `scim.*` de la auditoría—, así que no hay tabla
// nueva ni estado que mantener sincronizado. Una consulta con dos subconsultas
// y no dos viajes: esto se pinta en una tarjeta, no en un bucle.
func (s *Store) SCIMStatusForOrg(ctx context.Context, orgID string) (SCIMStatus, error) {
	const sql = `SELECT
		(SELECT count(*) FROM memberships m JOIN users u ON u.id = m.user_id
		   WHERE m.org_id = $1::uuid AND u.external_id IS NOT NULL),
		(SELECT max(occurred_at) FROM audit_log
		   WHERE org_id = $1::uuid AND action LIKE 'scim.%')`
	var st SCIMStatus
	err := s.DB().QueryRow(ctx, sql, orgID).Scan(&st.Provisioned, &st.LastEventAt)
	return st, errf("SCIMStatusForOrg", err)
}

// GetMembership devuelve la membresía concreta (rol) de un usuario en una org.
func (s *Store) GetMembership(ctx context.Context, orgID, userID string) (domain.Membership, error) {
	const sql = `SELECT org_id, user_id, role, created_at
		FROM memberships WHERE org_id=$1 AND user_id=$2`
	var m domain.Membership
	err := s.DB().QueryRow(ctx, sql, orgID, userID).
		Scan(&m.OrgID, &m.UserID, &m.Role, &m.CreatedAt)
	return m, errf("GetMembership", err)
}

// RolesForUserInOrg devuelve los roles (rol primario + derivados) del usuario.
func (s *Store) RolesForUserInOrg(ctx context.Context, orgID, userID string) ([]string, error) {
	m, err := s.GetMembership(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return []string{string(m.Role)}, nil
}

// SetOrgEducation marca o desmarca a una organización como institución
// educativa (mig 0064): el plan Pro le cuesta la mitad. Lo concede la
// plataforma, nunca la propia organización.
func (s *Store) SetOrgEducation(ctx context.Context, orgID string, education bool) (domain.Organization, error) {
	org, err := scanOrg(s.DB().QueryRow(ctx,
		`UPDATE organizations SET education=$2, updated_at=now() WHERE id=$1 RETURNING `+orgCols, orgID, education))
	return org, errf("SetOrgEducation", err)
}
