// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// El @usuario sale vacío mientras esté por asignar: quien entró por un binario
// anterior a la mig 0065 hasta que AsignarUsernamesPendientes pase por él.
const userCols = `id, email, coalesce(username, ''), email_verified, name, given_name, family_name,
	locale, coalesce(password_hash, ''), mfa_enabled, status,
	coalesce(external_id, ''), coalesce(department, ''), coalesce(title, ''),
	coalesce(location, ''), coalesce(phone, ''), coalesce(recovery_email, ''), created_at, updated_at,
	password_must_change`

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.EmailVerified, &u.Name, &u.GivenName,
		&u.FamilyName, &u.Locale, &u.PasswordHash, &u.MFAEnabled, &u.Status,
		&u.ExternalID, &u.Department, &u.Title, &u.Location, &u.Phone, &u.RecoveryEmail,
		&u.CreatedAt, &u.UpdatedAt, &u.PasswordMustChange)
	return u, err
}

// maxCarrerasUsername acota los reintentos cuando otra alta se queda con el mismo
// @usuario entre la lectura de los ocupados y la inserción. Perder más de un par
// de carreras seguidas no es mala suerte, es un bucle.
const maxCarrerasUsername = 5

// CreateUser inserta un usuario. email se normaliza a minúsculas.
//
// El @usuario: si viene (el alta autoservicio lo elige) se respeta, y si ya es de
// otra persona devuelve ErrConflict; si no viene, se le asigna el primero libre
// derivado de su nombre (ver usernameLibre). Va AQUÍ y no en cada vía de alta
// porque es el único sitio por el que pasan las seis.
func (s *Store) CreateUser(ctx context.Context, q pg.Querier, u domain.User) (domain.User, error) {
	if q == nil {
		q = s.DB()
	}
	// ON CONFLICT sobre el @usuario y SOLO sobre él: un choque de correo sigue
	// siendo un error, y uno de @usuario no puede abortar la transacción del alta
	// de quien llama —SCIM, una invitación— por una carrera con otra alta.
	const sql = `INSERT INTO users
		(email, username, email_verified, name, given_name, family_name, locale,
		 password_hash, mfa_enabled, status, external_id, department, title, location, phone)
		VALUES ($1,$2,$3,$4,$5,$6,$7,nullif($8,''),$9,$10,nullif($11,''),
		        nullif($12,''),nullif($13,''),nullif($14,''),nullif($15,''))
		ON CONFLICT ((lower(username))) DO NOTHING
		RETURNING ` + userCols
	elegido := u.Username != ""
	var ocupados map[string]bool
	for intento := 0; intento < maxCarrerasUsername; intento++ {
		username := u.Username
		if !elegido {
			var err error
			base := domain.UsernameBase(u.GivenName, u.FamilyName, u.Name, u.Email)
			if username, ocupados, err = s.usernameLibre(ctx, q, "", base, ocupados); err != nil {
				return domain.User{}, err
			}
		}
		got, err := scanUser(q.QueryRow(ctx, sql,
			strings.ToLower(strings.TrimSpace(u.Email)), username, u.EmailVerified,
			domain.SanitizeDisplayName(u.Name),
			domain.SanitizeDisplayName(u.GivenName), domain.SanitizeDisplayName(u.FamilyName),
			defaultStr(u.Locale, "es"), u.PasswordHash,
			u.MFAEnabled, defaultStatus(u.Status), u.ExternalID,
			domain.SanitizeDisplayName(u.Department), domain.SanitizeDisplayName(u.Title),
			domain.SanitizeDisplayName(u.Location), u.Phone))
		if !errors.Is(err, pgx.ErrNoRows) {
			return got, errf("CreateUser", err)
		}
		if elegido {
			return domain.User{}, fmt.Errorf("store: CreateUser: @usuario ocupado: %w", domain.ErrConflict)
		}
		ocupados[username] = true
	}
	return domain.User{}, fmt.Errorf("store: CreateUser: sin @usuario libre tras %d carreras: %w", maxCarrerasUsername, domain.ErrConflict)
}

// usernameLibre elige el primer @usuario libre para una raíz: ni el de otra
// persona ni la parte local de una dirección AJENA del dominio raíz, porque @ana
// es ana@sytue.eu y no pueden ser dos personas (ADR 0040). userID es la persona a
// la que se le asigna ("" si aún no existe): su propia dirección no le estorba.
//
// `ocupados` se devuelve para que quien reintenta tras perder una carrera no
// vuelva a leer lo que ya sabe; nil en la primera llamada.
func (s *Store) usernameLibre(ctx context.Context, q pg.Querier, userID, base string, ocupados map[string]bool) (string, map[string]bool, error) {
	// Una raíz reservada lo es con cualquier número detrás («info2» pliega a
	// «info»), y una vacía no da nada: las dos caen a una raíz neutra.
	if base == "" || domain.UsernameReservado(base) {
		base = "usuario"
	}
	if ocupados == nil {
		ocupados = map[string]bool{}
		// La raíz solo lleva [a-z0-9.-] (normalizarLocal), así que no trae
		// comodines de LIKE y el prefijo se puede usar tal cual.
		rows, err := q.Query(ctx, `
			SELECT lower(username) FROM users WHERE lower(username) LIKE $1 || '%'
			UNION
			SELECT lower(split_part(address, '@', 1)) FROM org_mailboxes
			 WHERE lower(address) LIKE $1 || '%@' || $2 AND removed_at IS NULL
			   AND user_id::text <> $3`,
			base, strings.ToLower(s.suiteDomain), userID)
		if err != nil {
			return "", nil, errf("usernameLibre", err)
		}
		for rows.Next() {
			var u string
			if err := rows.Scan(&u); err != nil {
				rows.Close()
				return "", nil, errf("usernameLibre.scan", err)
			}
			ocupados[u] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return "", nil, errf("usernameLibre.rows", err)
		}
	}
	// Termina: cada vuelta descarta un candidato distinto y los ocupados son finitos.
	for n := 0; ; n++ {
		c := domain.UsernameCandidate(base, n)
		if domain.ValidUsername(c) && !ocupados[c] {
			return c, ocupados, nil
		}
	}
}

// GetUserByAddress encuentra a la persona por cualquier dirección suya: primero
// un buzón o alias de Sytue —la dirección que la gente se escribe— y, si no hay,
// su correo de acceso. En ese orden porque el buzón es único en toda la
// plataforma y dice exactamente de quién es; el de acceso solo lo es entre
// cuentas.
func (s *Store) GetUserByAddress(ctx context.Context, address string) (domain.User, error) {
	const sql = `SELECT ` + userCols + ` FROM users WHERE id = (
		SELECT user_id FROM org_mailboxes WHERE lower(address) = lower($1) AND removed_at IS NULL)`
	u, err := scanUser(s.DB().QueryRow(ctx, sql, strings.TrimSpace(address)))
	if errors.Is(err, pgx.ErrNoRows) {
		return s.GetUserByEmail(ctx, address)
	}
	return u, errf("GetUserByAddress", err)
}

// UsernameLibre dice si un @usuario elegido (ya normalizado) está libre: no es el
// de otra persona ni la parte local de una dirección del dominio raíz, ya sea de
// un buzón o un correo de acceso. Es la pregunta de antes de mandar el SMS del
// alta; la respuesta definitiva la da el índice único al crear la cuenta.
func (s *Store) UsernameLibre(ctx context.Context, username string) (bool, error) {
	var libre bool
	dir := strings.ToLower(username) + "@" + strings.ToLower(s.suiteDomain)
	err := s.DB().QueryRow(ctx, `SELECT
		NOT EXISTS (SELECT 1 FROM users WHERE lower(username) = lower($1) OR lower(email) = $2)
		AND NOT EXISTS (SELECT 1 FROM org_mailboxes WHERE lower(address) = $2 AND removed_at IS NULL)`,
		username, dir).Scan(&libre)
	return libre, errf("UsernameLibre", err)
}

// GetUserByPhone recupera un usuario por su móvil (E.164, ya normalizado).
func (s *Store) GetUserByPhone(ctx context.Context, phone string) (domain.User, error) {
	const sql = `SELECT ` + userCols + ` FROM users WHERE phone = $1`
	u, err := scanUser(s.DB().QueryRow(ctx, sql, phone))
	return u, errf("GetUserByPhone", err)
}

// GetUserByRecoveryEmail recupera un usuario por su correo de recuperación.
func (s *Store) GetUserByRecoveryEmail(ctx context.Context, email string) (domain.User, error) {
	const sql = `SELECT ` + userCols + ` FROM users WHERE lower(recovery_email) = lower($1)`
	u, err := scanUser(s.DB().QueryRow(ctx, sql, strings.TrimSpace(email)))
	return u, errf("GetUserByRecoveryEmail", err)
}

// SetRecoveryEmail fija ("" retira) el correo de recuperación. ErrConflict si ya
// es el de recuperación de otra cuenta.
func (s *Store) SetRecoveryEmail(ctx context.Context, q pg.Querier, userID, email string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `UPDATE users SET recovery_email = nullif($2, '') WHERE id = $1`,
		userID, strings.ToLower(strings.TrimSpace(email)))
	return errf("SetRecoveryEmail", err)
}

// GetUserByUsername recupera un usuario por su @usuario (sin distinguir mayúsculas).
func (s *Store) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	const sql = `SELECT ` + userCols + ` FROM users WHERE lower(username) = lower($1)`
	u, err := scanUser(s.DB().QueryRow(ctx, sql, domain.NormalizeUsername(username)))
	return u, errf("GetUserByUsername", err)
}

// AsignarUsernamesPendientes da @usuario a quien todavía no lo tiene: todas las
// cuentas anteriores a la mig 0065 y las que cree un binario anterior durante el
// despliegue. Devuelve cuántas asignó en este lote.
//
// Van PRIMERO quienes tienen buzón en el dominio raíz, con su parte local tal
// cual: esa dirección ya es suya y no puede acabar siendo el @usuario de otra
// persona que pasara antes por el barrido.
func (s *Store) AsignarUsernamesPendientes(ctx context.Context, lote int) (int, error) {
	rows, err := s.DB().Query(ctx, `
		SELECT u.id::text, u.email, u.name, u.given_name, u.family_name,
		       coalesce(split_part(mb.address, '@', 1), '')
		FROM users u
		LEFT JOIN org_mailboxes mb
		       ON mb.user_id = u.id AND mb.primary_address AND mb.removed_at IS NULL
		      AND lower(split_part(mb.address, '@', 2)) = lower($1)
		-- Una cuenta purgada (mig 0098) perdió su @usuario a propósito: no se le
		-- vuelve a dar uno.
		WHERE u.username IS NULL AND u.purged_at IS NULL
		ORDER BY (mb.address IS NULL), u.created_at, u.id
		LIMIT $2`, s.suiteDomain, lote)
	if err != nil {
		return 0, errf("AsignarUsernamesPendientes", err)
	}
	type pendiente struct{ id, email, name, given, family, raiz string }
	var lista []pendiente
	for rows.Next() {
		var p pendiente
		if err := rows.Scan(&p.id, &p.email, &p.name, &p.given, &p.family, &p.raiz); err != nil {
			rows.Close()
			return 0, errf("AsignarUsernamesPendientes.scan", err)
		}
		lista = append(lista, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, errf("AsignarUsernamesPendientes.rows", err)
	}

	hechos := 0
	for _, p := range lista {
		base := domain.NormalizeUsername(p.raiz)
		if !domain.ValidUsername(base) {
			base = domain.UsernameBase(p.given, p.family, p.name, p.email)
		}
		var ocupados map[string]bool
		for intento := 0; intento < maxCarrerasUsername; intento++ {
			var cand string
			if cand, ocupados, err = s.usernameLibre(ctx, s.DB(), p.id, base, ocupados); err != nil {
				return hechos, err
			}
			tag, err := s.DB().Exec(ctx,
				`UPDATE users SET username = $2 WHERE id = $1 AND username IS NULL`, p.id, cand)
			if errors.Is(pgErr(err), domain.ErrConflict) {
				// Otra réplica, o un alta, se lo quedó entre la lectura y aquí.
				ocupados[cand] = true
				continue
			}
			if err != nil {
				return hechos, errf("AsignarUsernamesPendientes.update", err)
			}
			if tag.RowsAffected() > 0 {
				hechos++
			}
			break
		}
	}
	return hechos, nil
}

// GetUserByID recupera un usuario por id.
func (s *Store) GetUserByID(ctx context.Context, id string) (domain.User, error) {
	const sql = `SELECT ` + userCols + ` FROM users WHERE id = $1`
	u, err := scanUser(s.DB().QueryRow(ctx, sql, id))
	return u, errf("GetUserByID", err)
}

// GetUserByEmail recupera un usuario por email (case-insensitive).
func (s *Store) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	const sql = `SELECT ` + userCols + ` FROM users WHERE lower(email) = lower($1)`
	u, err := scanUser(s.DB().QueryRow(ctx, sql, strings.TrimSpace(email)))
	return u, errf("GetUserByEmail", err)
}

// GetUserByExternalID recupera un usuario por su external_id de SCIM.
func (s *Store) GetUserByExternalID(ctx context.Context, ext string) (domain.User, error) {
	const sql = `SELECT ` + userCols + ` FROM users WHERE external_id = $1`
	u, err := scanUser(s.DB().QueryRow(ctx, sql, ext))
	return u, errf("GetUserByExternalID", err)
}

// UpdateUser actualiza campos de perfil/estado.
//
// Si el correo CAMBIA, deja de constar como verificado, diga lo que diga quien
// llama: `email_verified` afirma que alguien demostró control de ESA dirección, y de
// la nueva no lo ha demostrado nadie. Es la única vía que cambia el correo de una
// cuenta (la recorre SCIM, con PUT y PATCH de `userName`), y sin esto la dirección
// nueva heredaba el «verificado» de la antigua y salía así en el claim `email` de
// los tokens, que varios servicios usan como identidad. En el SET de PostgreSQL,
// `email` a la derecha es el valor ANTERIOR de la fila, que es justo lo que se
// compara.
func (s *Store) UpdateUser(ctx context.Context, q pg.Querier, u domain.User) (domain.User, error) {
	if s.needsExternalAccessTransaction(q) {
		var out domain.User
		err := s.InTx(ctx, func(tx pg.Querier) (err error) {
			out, err = s.UpdateUser(ctx, tx, u)
			return err
		})
		return out, err
	}
	if err := s.suspendExternalAccount(ctx, q, u.ID, defaultStatus(u.Status)); err != nil {
		return domain.User{}, err
	}
	const sql = `UPDATE users SET
		email=$2,
		email_verified = CASE WHEN lower(email) = lower($2) THEN $3 ELSE false END,
		name=$4, given_name=$5, family_name=$6,
		locale=$7, mfa_enabled=$8, status=$9,
		department=nullif($10,''), title=nullif($11,''), location=nullif($12,'')
		WHERE id=$1 AND status <> 'deleted' RETURNING ` + userCols
	got, err := scanUser(q.QueryRow(ctx, sql, u.ID,
		strings.ToLower(strings.TrimSpace(u.Email)), u.EmailVerified,
		domain.SanitizeDisplayName(u.Name),
		domain.SanitizeDisplayName(u.GivenName), domain.SanitizeDisplayName(u.FamilyName),
		defaultStr(u.Locale, "es"), u.MFAEnabled,
		defaultStatus(u.Status),
		domain.SanitizeDisplayName(u.Department), domain.SanitizeDisplayName(u.Title),
		domain.SanitizeDisplayName(u.Location)))
	if errors.Is(err, pgx.ErrNoRows) {
		// El `AND status <> 'deleted'` del WHERE es la verja ANTI-RESURRECCIÓN: sin él,
		// un PATCH/PUT de SCIM con active=true devolvía a la vida una cuenta dada de
		// baja (SetUserStatus sí tenía el guard; esta función no). Una cuenta borrada
		// ya no existe para el aprovisionamiento: 404, no 500.
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return got, errf("UpdateUser", err)
	}
	// Es la vía por la que SCIM aplica `active:false` (status suspended), así que la
	// cascada de revocación tiene que pasar también por aquí.
	if err := revokeIfInactive(ctx, s, q, got.ID, got.Status); err != nil {
		return got, err
	}
	if got.Status == domain.StatusActive {
		if err := s.queueExternalActivation(ctx, q, got.ID, ""); err != nil {
			return got, err
		}
	}
	return got, nil
}

// SetDisplayName actualiza SOLO el nombre visible del usuario (perfil self-service).
// A propósito toca únicamente la columna name: email/rol/tenant/estado quedan
// intactos a nivel SQL, de modo que este camino no puede alterarlos (a diferencia
// de UpdateUser, que reescribe todo el registro). Devuelve el usuario actualizado.
func (s *Store) SetDisplayName(ctx context.Context, q pg.Querier, userID, name string) (domain.User, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE users SET name=$2 WHERE id=$1 RETURNING ` + userCols
	u, err := scanUser(q.QueryRow(ctx, sql, userID, domain.SanitizeDisplayName(name)))
	return u, errf("SetDisplayName", err)
}

// SetProfile actualiza los campos de perfil que el propio usuario puede editar
// desde /account: nombre visible, nombre y apellidos, puesto, departamento,
// ubicación e idioma.
//
// Enumera columnas por la misma razón que SetDisplayName: UpdateUser reescribe
// el registro entero —incluidos email_verified, status y mfa_enabled—, así que
// dárselo a la cara self-service convertiría un formulario de perfil en una vía
// para verificarse el correo o reactivarse una cuenta suspendida. Aquí esas
// columnas no aparecen en el SQL y por tanto no hay campo que forzar.
//
// El WHERE repite el guard anti-resurrección de UpdateUser: una cuenta borrada
// no edita su perfil.
func (s *Store) SetProfile(ctx context.Context, q pg.Querier, userID string, p domain.ProfileEdit) (domain.User, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE users SET
		name=$2, given_name=$3, family_name=$4,
		department=nullif($5,''), title=nullif($6,''), location=nullif($7,''),
		locale=$8
		WHERE id=$1 AND status <> 'deleted' RETURNING ` + userCols
	u, err := scanUser(q.QueryRow(ctx, sql, userID,
		domain.SanitizeDisplayName(p.Name),
		domain.SanitizeDisplayName(p.GivenName), domain.SanitizeDisplayName(p.FamilyName),
		domain.SanitizeDisplayName(p.Department), domain.SanitizeDisplayName(p.Title),
		domain.SanitizeDisplayName(p.Location),
		domain.NormalizeLocale(p.Locale)))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	return u, errf("SetProfile", err)
}

// SetPassword fija el hash de contraseña (Argon2id) de un usuario. Apaga la
// marca de contraseña temporal: la elija quien la elija por esta vía, ya no es
// la que puso un operador.
func (s *Store) SetPassword(ctx context.Context, q pg.Querier, userID, hash string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `UPDATE users SET password_hash=$2, password_must_change=false WHERE id=$1`, userID, hash)
	return errf("SetPassword", err)
}

// SetTemporaryPassword fija una contraseña que obliga a cambiarse en el
// siguiente inicio de sesión (mig 0113). La pone un operador de plataforma.
func (s *Store) SetTemporaryPassword(ctx context.Context, q pg.Querier, userID, hash string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `UPDATE users SET password_hash=$2, password_must_change=true WHERE id=$1`, userID, hash)
	return errf("SetTemporaryPassword", err)
}

// SetMFAEnabled marca el flag de MFA del usuario.
func (s *Store) SetMFAEnabled(ctx context.Context, q pg.Querier, userID string, enabled bool) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `UPDATE users SET mfa_enabled=$2 WHERE id=$1`, userID, enabled)
	return errf("SetMFAEnabled", err)
}

// ListUsers pagina usuarios (SCIM ListResponse). filterEmail opcional (igualdad).
func (s *Store) ListUsers(ctx context.Context, filterEmail string, offset, limit int) ([]domain.User, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{}
	where := ""
	if filterEmail != "" {
		where = ` WHERE lower(email) = lower($1)`
		args = append(args, filterEmail)
	}
	var total int
	if err := s.DB().QueryRow(ctx, `SELECT count(*) FROM users`+where, args...).Scan(&total); err != nil {
		return nil, 0, errf("ListUsers.count", err)
	}
	args = append(args, offset, limit)
	pg1, pg2 := len(args)-1, len(args)
	sql := `SELECT ` + userCols + ` FROM users` + where +
		` ORDER BY created_at OFFSET $` + itoa(pg1) + ` LIMIT $` + itoa(pg2)
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, errf("ListUsers", err)
	}
	defer rows.Close()
	var out []domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, errf("ListUsers.scan", err)
		}
		out = append(out, u)
	}
	return out, total, errf("ListUsers.iter", rows.Err())
}

// ListOrgUsers es ListUsers ACOTADO a los miembros de orgID: mismo filtro/paginación
// offset (SCIM), pero solo usuarios con membresía en la org — nunca devuelve usuarios
// de otro tenant (cierra la fuga de PII cross-tenant del SCIM ligado a una org). Usa
// subconsulta IN sobre memberships (no JOIN) para reusar userCols sin ambigüedad.
func (s *Store) ListOrgUsers(ctx context.Context, orgID, filterEmail string, offset, limit int) ([]domain.User, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{orgID}
	where := ` WHERE id IN (SELECT user_id FROM memberships WHERE org_id = $1)`
	if filterEmail != "" {
		where += ` AND lower(email) = lower($2)`
		args = append(args, filterEmail)
	}
	var total int
	if err := s.DB().QueryRow(ctx, `SELECT count(*) FROM users`+where, args...).Scan(&total); err != nil {
		return nil, 0, errf("ListOrgUsers.count", err)
	}
	args = append(args, offset, limit)
	pg1, pg2 := len(args)-1, len(args)
	sql := `SELECT ` + userCols + ` FROM users` + where +
		` ORDER BY created_at OFFSET $` + itoa(pg1) + ` LIMIT $` + itoa(pg2)
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, errf("ListOrgUsers", err)
	}
	defer rows.Close()
	var out []domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, errf("ListOrgUsers.scan", err)
		}
		out = append(out, u)
	}
	return out, total, errf("ListOrgUsers.iter", rows.Err())
}

// UserCursor es la posición keyset (created_at, id) del último usuario devuelto
// por ListOrgUsersPage. nil ⇒ primera página.
type UserCursor struct {
	CreatedAt time.Time
	ID        string
}

// OrgUser es una fila del directorio admin: el usuario con su rol en la org.
// Deliberadamente sin password_hash ni otros campos sensibles.
//
// Departamento, puesto y ubicación viajan con la fila porque son lo que evalúan
// las reglas de los grupos dinámicos: sin ellos la consola puede escribirlos pero
// no enseñar cuáles hay ahora, y un campo en blanco sobre alguien que SÍ tiene
// departamento se lee como «no tiene» — y se corrige lo que estaba bien.
type OrgUser struct {
	ID            string
	Email         string
	EmailVerified bool
	Name          string
	MFAEnabled    bool
	Status        domain.Status
	CreatedAt     time.Time
	Role          string
	Department    string
	Title         string
	Location      string
}

// ListOrgUsersPage lista los usuarios miembros de una organización ordenados por
// (created_at, id) DESC con paginación keyset — WHERE (created_at, id) < ($ts, $id),
// sin OFFSET; el índice users_created_at_id_desc_idx (0007) sirve el ORDER BY y
// el WHERE como Index Scan a cientos de miles de usuarios. Devuelve hasta limit
// filas y more=true si queda al menos una página más (consulta limit+1).
//
// `q` filtra por subcadena de correo o nombre contra la columna generada
// search_text (mig 0044, índice GIN de trigramas). Va AQUÍ y no en el cliente
// porque el listado está paginado: filtrar lo ya cargado convierte «no está en
// esta organización» y «no está en las primeras cincuenta filas» en la misma
// respuesta vacía. El keyset sigue valiendo con el filtro puesto: el orden no
// cambia, solo se recorren menos filas.
//
// Endpoint admin de Console. El listado SCIM usa ListUsers (startIndex/count,
// RFC 7644) y NO debe tocarse.
func (s *Store) ListOrgUsersPage(ctx context.Context, orgID, q string, after *UserCursor, limit int) ([]OrgUser, bool, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// ponytail: plan esperado Index Scan (users por el índice 0007) + nested loop
	// sobre memberships_pkey; si una org minoritaria en una tabla enorme lo vuelve
	// caro, denormalizar created_at en memberships con su propio índice compuesto.
	const base = `SELECT u.id, u.email, u.email_verified, u.name, u.mfa_enabled,
		u.status, u.created_at, m.role,
		coalesce(u.department, ''), coalesce(u.title, ''), coalesce(u.location, '')
		FROM users u JOIN memberships m ON m.org_id = $1 AND m.user_id = u.id`
	args := []any{orgID}
	var conds []string
	if after != nil {
		args = append(args, after.CreatedAt, after.ID)
		conds = append(conds, fmt.Sprintf("(u.created_at, u.id) < ($%d::timestamptz, $%d::uuid)", len(args)-1, len(args)))
	}
	// search_text ya viene plegado de la columna generada, así que el término
	// también: un LIKE que usa el índice de trigramas, no un plegar() por fila.
	if term := strings.TrimSpace(q); term != "" {
		args = append(args, "%"+escapeLike(term)+"%")
		conds = append(conds, fmt.Sprintf(`u.search_text LIKE plegar($%d) ESCAPE '\'`, len(args)))
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit+1)
	sql := fmt.Sprintf("%s%s ORDER BY u.created_at DESC, u.id DESC LIMIT $%d", base, where, len(args))
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, false, errf("ListOrgUsersPage", err)
	}
	defer rows.Close()
	out := make([]OrgUser, 0, limit)
	for rows.Next() {
		var u OrgUser
		if err := rows.Scan(&u.ID, &u.Email, &u.EmailVerified, &u.Name,
			&u.MFAEnabled, &u.Status, &u.CreatedAt, &u.Role,
			&u.Department, &u.Title, &u.Location); err != nil {
			return nil, false, errf("ListOrgUsersPage.scan", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, false, errf("ListOrgUsersPage.iter", err)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// SetUserStatus cambia el estado de ciclo de vida de un usuario (active⇄suspended
// desde la Console admin). No resucita cuentas dadas de baja: status='deleted' es
// terminal. Devuelve true si actualizó una fila (usuario existente y no borrado).
// El estado lo consumen login (login.go) y refresh (token.go): suspender bloquea
// el acceso de inmediato, sin depender solo de la caducidad del access token.
func (s *Store) SetUserStatus(ctx context.Context, q pg.Querier, userID string, status domain.Status) (bool, error) {
	if s.needsExternalAccessTransaction(q) {
		var changed bool
		err := s.InTx(ctx, func(tx pg.Querier) (err error) {
			changed, err = s.SetUserStatus(ctx, tx, userID, status)
			return err
		})
		return changed, err
	}
	if err := s.suspendExternalAccount(ctx, q, userID, status); err != nil {
		return false, err
	}
	tag, err := q.Exec(ctx, `UPDATE users SET status=$2
		WHERE id=$1 AND status <> 'deleted'`, userID, status)
	if err != nil {
		return false, errf("SetUserStatus", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	// Dejar de estar activo ARRASTRA las credenciales (ver revokeIfInactive).
	if err := revokeIfInactive(ctx, s, q, userID, status); err != nil {
		return false, err
	}
	if status == domain.StatusActive {
		if err := s.queueExternalActivation(ctx, q, userID, ""); err != nil {
			return false, err
		}
	}
	return true, nil
}

// revokeIfInactive revoca sesiones SSO y refresh tokens cuando una cuenta deja de estar
// activa. Vive en el STORE a propósito: es el punto por el que convergen TODOS los
// caminos de baja (los tres verbos SCIM, la Console y cualquier futuro), así que la
// cascada no depende de que cada llamante se acuerde.
//
// Sin esto, el desaprovisionamiento por SCIM —el offboarding estándar de empresa, vía
// Entra u Okta— no revocaba NADA: la sesión SSO seguía viva y con ella se obtenían
// códigos, tokens y Assertions SAML firmadas hacia SPs de terceros, donde la caducidad
// la manda el SP. El camino de la Console sí revocaba (admin_users.go), y esa asimetría
// es la que se cierra aquí.
//
// Idempotente (las consultas filtran por `revoked_at IS NULL`), así que no molesta que
// un llamante revoque además por su cuenta.
func revokeIfInactive(ctx context.Context, s *Store, q pg.Querier, userID string, status domain.Status) error {
	if status == domain.StatusActive {
		return nil
	}
	if _, err := s.RevokeAllUserSessions(ctx, q, userID); err != nil {
		return err
	}
	return nil
}

// DeleteUser realiza un borrado lógico (status=deleted) preservando auditoría, y
// REVOCA las credenciales en cascada: sin ello el desaprovisionamiento SCIM
// (DELETE /scim/v2/Users/{id}) dejaba la sesión y los refresh tokens vivos.
func (s *Store) DeleteUser(ctx context.Context, q pg.Querier, id string) error {
	if s.needsExternalAccessTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error { return s.DeleteUser(ctx, tx, id) })
	}
	if err := s.suspendExternalAccount(ctx, q, id, domain.StatusDeleted); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `UPDATE users SET status='deleted' WHERE id=$1`, id); err != nil {
		return errf("DeleteUser", err)
	}
	return revokeIfInactive(ctx, s, q, id, domain.StatusDeleted)
}

// --- Baja y restauración de una cuenta (mig 0092) -----------------------------
//
// DeleteUser (arriba) es el camino de SCIM: marca `deleted` y punto. Estos dos
// son el de Sytue Admin, y se distinguen en que dejan escrito CUÁNDO, QUIÉN y
// POR QUÉ — sin eso no hay ventana de recuperación posible, y una restauración
// sin ventana significa que el borrado no significaba nada.

// ErrNoRecuperable es una baja que ya no se puede deshacer: o la fila no está
// dada de baja, o su ventana venció, o se dio de baja antes de que existieran
// estas columnas y no hay fecha con la que medir.
var ErrNoRecuperable = errors.New("la cuenta no se puede restaurar")

// CloseUser da de baja una cuenta dejando rastro, y revoca sus credenciales en
// la misma transacción. Devuelve false si la cuenta ya estaba dada de baja
// (idempotente: repetirlo no es un error, pero tampoco reescribe quién la dio).
func (s *Store) CloseUser(ctx context.Context, q pg.Querier, id, porID, motivo string) (bool, error) {
	if s.needsExternalAccessTransaction(q) {
		var changed bool
		err := s.InTx(ctx, func(tx pg.Querier) (err error) {
			changed, err = s.CloseUser(ctx, tx, id, porID, motivo)
			return err
		})
		return changed, err
	}
	if err := s.suspendExternalAccount(ctx, q, id, domain.StatusDeleted); err != nil {
		return false, err
	}
	tag, err := q.Exec(ctx, `UPDATE users
		   SET status='deleted', closed_at=now(), closed_by=$2::uuid, close_reason=$3,
		       personal_data_erased_at=NULL, purged_at=NULL
		 WHERE id=$1 AND status <> 'deleted'`, id, nilSiVacio(porID), motivo)
	if err != nil {
		return false, errf("CloseUser", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, revokeIfInactive(ctx, s, q, id, domain.StatusDeleted)
}

// RestoreUser deshace una baja dentro de su ventana y deja la cuenta SUSPENDIDA,
// no activa.
//
// Es deliberado y es la mitad del valor de esto: restaurar devuelve los datos,
// no el acceso. Quien administra decide después si reactiva, y mientras tanto
// nadie entra con una cuenta que ayer estaba dada de baja. Reactivar ya tiene su
// propio camino auditado (SetUserStatus), y este no lo atajará nunca.
//
// Una baja PROPIA («Borrar mi cuenta», closed_by = la cuenta) no se restaura
// por aquí: es la supresión que pidió la persona (art. 17 RGPD), y un
// administrador de cualquiera de sus organizaciones no puede anularla. Solo la
// deshace el equipo de privacidad, a petición suya (ADR 0054).
//
// `desde` es el límite de la ventana: una baja anterior a esa marca ya no se
// toca. La comprobación va en el WHERE —no en un SELECT previo— para que dos
// peticiones a la vez no restauren dos veces.
func (s *Store) RestoreUser(ctx context.Context, q pg.Querier, id string, desde time.Time) error {
	if s.needsCallIdentityTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error { return s.RestoreUser(ctx, tx, id, desde) })
	}
	if q == nil {
		q = s.DB()
	}
	return restaurarComo(ctx, q, id, desde, domain.StatusSuspended, "RestoreUser")
}

// ReactivateClosedUser deshace una baja y deja la cuenta ACTIVA en un solo
// gesto. Es lo que hace «Reactivar» en la consola sobre una cuenta dada de
// baja: quien pulsa reactivar ya está decidiendo devolver el acceso, y pedirle
// antes un «restaurar» que no ve en el directorio era dejarle sin salida
// («Aplicado a 0 personas»). Mismas reglas que RestoreUser: ventana y baja
// propia (RGPD) siguen en pie.
//
// En transacción, como SetUserStatus: volver a activa cambia la identidad que ven las
// llamadas de documentos (mig 0117), y sin la transacción el guardián de la base la
// rechazaba entera («Document calls must be revoked…», un 500 en «Reactivar»). Y como
// SetUserStatus, encola la reactivación en las cuentas externas.
func (s *Store) ReactivateClosedUser(ctx context.Context, q pg.Querier, id string, desde time.Time) error {
	if s.needsExternalAccessTransaction(q) {
		return s.InTx(ctx, func(tx pg.Querier) error { return s.ReactivateClosedUser(ctx, tx, id, desde) })
	}
	if err := restaurarComo(ctx, q, id, desde, domain.StatusActive, "ReactivateClosedUser"); err != nil {
		return err
	}
	return s.queueExternalActivation(ctx, q, id, "")
}

// restaurarComo es el UPDATE común. Una baja SIN fecha (`closed_at IS NULL`:
// el desaprovisionamiento SCIM, o anterior a la mig 0092) también se admite:
// sin fecha no hay ventana que haya vencido, y negarla dejaba esas cuentas
// irrecuperables para siempre sin que nadie lo hubiera decidido.
func restaurarComo(ctx context.Context, q pg.Querier, id string, desde time.Time, st domain.Status, op string) error {
	tag, err := q.Exec(ctx, `UPDATE users
		   SET status=$3, closed_at=NULL, closed_by=NULL, close_reason=''
		 WHERE id=$1 AND status='deleted' AND (closed_at IS NULL OR closed_at >= $2)
		   AND closed_by IS DISTINCT FROM id`,
		id, desde, st)
	if err != nil {
		return errf(op, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRecuperable
	}
	return nil
}

// CuentaCerrada es una baja recuperable tal como la enseña la consola.
type CuentaCerrada struct {
	ID       string
	Email    string
	Name     string
	ClosedAt time.Time
	ClosedBy string
	Reason   string
}

// ClosedUsersInOrg lista las bajas de una organización posteriores a `desde`
// —las que todavía se pueden deshacer—, más recientes primero.
//
// Acotado por la membresía: una baja no borra la fila de `memberships`, que es
// justo lo que permite saber de qué organización era. Tope de 200: si una
// organización tiene más bajas vivas que eso, lo que hace falta es un filtro,
// no una página más.
func (s *Store) ClosedUsersInOrg(ctx context.Context, orgID string, desde time.Time) ([]CuentaCerrada, error) {
	const sql = `SELECT u.id, u.email, u.name, u.closed_at,
		coalesce(quien.email, ''), u.close_reason
		FROM users u
		JOIN memberships m ON m.user_id = u.id AND m.org_id = $1
		LEFT JOIN users quien ON quien.id = u.closed_by
		WHERE u.status = 'deleted' AND u.closed_at IS NOT NULL AND u.closed_at >= $2
		  AND u.closed_by IS DISTINCT FROM u.id
		ORDER BY u.closed_at DESC
		LIMIT 200`
	rows, err := s.DB().Query(ctx, sql, orgID, desde)
	if err != nil {
		return nil, errf("ClosedUsersInOrg", err)
	}
	defer rows.Close()
	out := make([]CuentaCerrada, 0, 16)
	for rows.Next() {
		var c CuentaCerrada
		if err := rows.Scan(&c.ID, &c.Email, &c.Name, &c.ClosedAt, &c.ClosedBy, &c.Reason); err != nil {
			return nil, errf("ClosedUsersInOrg.scan", err)
		}
		out = append(out, c)
	}
	return out, errf("ClosedUsersInOrg.iter", rows.Err())
}

// nilSiVacio convierte "" en NULL: `closed_by` es uuid y una cadena vacía no
// castea. Pasa con una baja que dispara un proceso y no una persona.
func nilSiVacio(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func defaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func defaultStatus(s domain.Status) domain.Status {
	if s == "" {
		return domain.StatusActive
	}
	return s
}
