// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// --- Códigos de autorización (un solo uso) ---

// InsertAuthCode persiste un código de autorización (solo el hash).
func (s *Store) InsertAuthCode(ctx context.Context, q pg.Querier, c domain.AuthCode) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO auth_codes
		(code_hash, client_id, user_id, org_id, session_id, redirect_uri, scope, nonce,
		 code_challenge, code_challenge_method, auth_time, expires_at, dispositivo)
		VALUES ($1,$2,$3,nullif($4,'')::uuid,nullif($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12,$13)`
	_, err := q.Exec(ctx, sql, c.CodeHash, c.ClientID, c.UserID, c.OrgID, c.SessionID,
		c.RedirectURI, c.Scope, c.Nonce, c.CodeChallenge, c.CodeChallengeMethod,
		c.AuthTime, c.ExpiresAt, c.Dispositivo)
	return errf("InsertAuthCode", err)
}

// ConsumeAuthCode recupera y marca consumido un código en una sola operación
// atómica (defensa anti-replay): solo devuelve fila si no estaba consumido ni
// expirado. Debe ejecutarse dentro de una transacción.
func (s *Store) ConsumeAuthCode(ctx context.Context, q pg.Querier, codeHash string, now time.Time) (domain.AuthCode, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE auth_codes SET consumed_at = $2
		WHERE code_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING code_hash, client_id, user_id, coalesce(org_id::text,''),
		          coalesce(session_id::text,''), redirect_uri, scope, nonce, code_challenge,
		          code_challenge_method, auth_time, expires_at, dispositivo`
	var c domain.AuthCode
	err := q.QueryRow(ctx, sql, codeHash, now).Scan(
		&c.CodeHash, &c.ClientID, &c.UserID, &c.OrgID, &c.SessionID, &c.RedirectURI,
		&c.Scope, &c.Nonce, &c.CodeChallenge, &c.CodeChallengeMethod,
		&c.AuthTime, &c.ExpiresAt, &c.Dispositivo,
	)
	return c, errf("ConsumeAuthCode", err)
}

// --- Consentimiento de las apps nativas (NAT-02, mig 0114) ---

// ConsentidaEnDispositivo dice si la persona ya dio acceso a esa app nativa (client y
// vuelta) en ese dispositivo.
func (s *Store) ConsentidaEnDispositivo(ctx context.Context, userID, clientID, redirectURI, dispositivo string) (bool, error) {
	var ok bool
	err := s.DB().QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM consentimientos_nativos
		WHERE user_id = $1 AND client_id = $2 AND redirect_uri = $3 AND dispositivo = $4)`,
		userID, clientID, redirectURI, dispositivo).Scan(&ok)
	return ok, errf("ConsentidaEnDispositivo", err)
}

// RecordarConsentimiento apunta que la persona dio acceso a esa app en ese dispositivo.
func (s *Store) RecordarConsentimiento(ctx context.Context, userID, clientID, redirectURI, dispositivo string) error {
	_, err := s.DB().Exec(ctx, `INSERT INTO consentimientos_nativos (user_id, client_id, redirect_uri, dispositivo)
		VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, userID, clientID, redirectURI, dispositivo)
	return errf("RecordarConsentimiento", err)
}

// --- Sesiones del Authorization Server ---

// InsertSession crea una sesión SSO (cookie httpOnly).
func (s *Store) InsertSession(ctx context.Context, q pg.Querier, ss domain.Session) (string, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO sessions
		(token_hash, user_id, org_id, auth_time, amr, user_agent, lugar_cifrado, expires_at)
		VALUES ($1,$2,nullif($3,'')::uuid,$4,$5,$6,$7,$8)
		RETURNING id`
	var id string
	err := q.QueryRow(ctx, sql, ss.TokenHash, ss.UserID, ss.OrgID, ss.AuthTime,
		ss.AMR, ss.UserAgent, ss.LugarCifrado, ss.ExpiresAt).Scan(&id)
	return id, errf("InsertSession", err)
}

// GetSessionByToken recupera una sesión vigente por hash de token.
func (s *Store) GetSessionByToken(ctx context.Context, tokenHash string) (domain.Session, error) {
	const sql = `SELECT id, token_hash, user_id, coalesce(org_id::text,''),
		auth_time, amr, user_agent, lugar_cifrado, expires_at, revoked_at
		FROM sessions WHERE token_hash = $1`
	var ss domain.Session
	err := s.DB().QueryRow(ctx, sql, tokenHash).Scan(&ss.ID, &ss.TokenHash,
		&ss.UserID, &ss.OrgID, &ss.AuthTime, &ss.AMR, &ss.UserAgent, &ss.LugarCifrado,
		&ss.ExpiresAt, &ss.RevokedAt)
	return ss, errf("GetSessionByToken", err)
}

// SessionAMR devuelve los métodos de autenticación con los que se abrió una
// sesión. Existe porque el `amr` de los tokens NO puede inventarse: es la claim
// con la que un relying party decide si exige un segundo paso, y el endpoint de
// token la fijaba a «pwd» a secas pasara lo que pasara en el login. Eso hacía
// insatisfacible cualquier política que pidiera MFA —la consola de plataforma,
// sin ir más lejos— porque el token siempre decía que solo hubo contraseña.
//
// Sesión vacía o inexistente ⇒ nada, y el llamante decide: es preferible un
// token sin `amr` a uno que afirme algo que no ocurrió.
func (s *Store) SessionAMR(ctx context.Context, sessionID string) ([]string, error) {
	if sessionID == "" {
		return nil, nil
	}
	var amr []string
	err := s.DB().QueryRow(ctx,
		`SELECT amr FROM sessions WHERE id = $1::uuid AND revoked_at IS NULL`,
		sessionID).Scan(&amr)
	if err != nil {
		return nil, errf("SessionAMR", err)
	}
	return amr, nil
}

// RevokeSession revoca una sesión (logout).
func (s *Store) RevokeSession(ctx context.Context, q pg.Querier, tokenHash string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `UPDATE sessions SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	return errf("RevokeSession", err)
}

// ListSessionsForUser devuelve las sesiones SSO vigentes (ni revocadas ni
// caducadas) de un usuario, más recientes primero, marcando Current en la que
// coincide con currentTokenHash (la sesión desde la que se consulta; "" no
// marca ninguna). No expone token_hash. Acotada a 100 filas: ningún usuario
// legítimo mantiene más sesiones vivas y evita cargas sin límite.
func (s *Store) ListSessionsForUser(ctx context.Context, userID, currentTokenHash string) ([]domain.Session, error) {
	const sql = `SELECT id, lugar_cifrado, user_agent, amr, auth_time,
			expires_at, token_hash = $2
		FROM sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		ORDER BY auth_time DESC
		LIMIT 100`
	rows, err := s.DB().Query(ctx, sql, userID, currentTokenHash)
	if err != nil {
		return nil, errf("ListSessionsForUser", err)
	}
	defer rows.Close()
	var out []domain.Session
	for rows.Next() {
		ss := domain.Session{UserID: userID}
		if err := rows.Scan(&ss.ID, &ss.LugarCifrado, &ss.UserAgent, &ss.AMR,
			&ss.AuthTime, &ss.ExpiresAt, &ss.Current); err != nil {
			return nil, errf("ListSessionsForUser.scan", err)
		}
		out = append(out, ss)
	}
	return out, errf("ListSessionsForUser.iter", rows.Err())
}

// ListPastSessionsForUser devuelve las sesiones ya terminadas —revocadas o
// caducadas— del usuario, más recientes primero. Es el complemento de
// ListSessionsForUser: sin él, «cerrar sesión» borra la única prueba de que un
// acceso ocurrió, y quien sospeche de un acceso ajeno no tiene dónde mirar.
// RevokedAt viene relleno para poder distinguir en la interfaz una sesión que
// se cerró de una que expiró sola.
//
// Acotada a las 50 últimas: el registro completo y permanente de accesos es la
// auditoría, no esta lista, que solo sirve para reconocer lo reciente.
func (s *Store) ListPastSessionsForUser(ctx context.Context, userID string) ([]domain.Session, error) {
	const sql = `SELECT id, lugar_cifrado, user_agent, amr, auth_time,
			expires_at, revoked_at
		FROM sessions
		WHERE user_id = $1 AND (revoked_at IS NOT NULL OR expires_at <= now())
		ORDER BY auth_time DESC
		LIMIT 50`
	rows, err := s.DB().Query(ctx, sql, userID)
	if err != nil {
		return nil, errf("ListPastSessionsForUser", err)
	}
	defer rows.Close()
	var out []domain.Session
	for rows.Next() {
		ss := domain.Session{UserID: userID}
		if err := rows.Scan(&ss.ID, &ss.LugarCifrado, &ss.UserAgent, &ss.AMR,
			&ss.AuthTime, &ss.ExpiresAt, &ss.RevokedAt); err != nil {
			return nil, errf("ListPastSessionsForUser.scan", err)
		}
		out = append(out, ss)
	}
	return out, errf("ListPastSessionsForUser.iter", rows.Err())
}

// RevokeSessionByID revoca una sesión concreta del usuario (cerrar un
// dispositivo desde /account) y, en cascada, los refresh tokens ligados a ella
// (encadena RevokeRefreshTokensForSession, igual que el logout). El guard
// user_id impide revocar sesiones ajenas (anti-IDOR); si la sesión no es del
// usuario o ya estaba revocada devuelve (false, nil) sin tocar nada.
// Ejecutar dentro de una transacción para que sesión y refresh caigan juntos.
func (s *Store) RevokeSessionByID(ctx context.Context, q pg.Querier, userID, sessionID string) (bool, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `UPDATE sessions SET revoked_at = now()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, sessionID, userID)
	if err != nil {
		return false, errf("RevokeSessionByID", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, s.RevokeRefreshTokensForSession(ctx, q, sessionID)
}

// RevokeOtherSessions revoca todas las sesiones vigentes del usuario salvo la
// indicada (la actual) y encadena RevokeRefreshTokensForSession por cada una
// (revocación en cascada, como el logout). Devuelve cuántas cerró. Ejecutar
// dentro de una transacción para que sesiones y refresh caigan juntos.
//
// `keepSessionID` VACÍO significa «no conserves ninguna»: se cierran todas. Antes
// no significaba nada, porque `id <> $2` obligaba a Postgres a leer `$2` como
// `uuid`, y la cadena vacía no lo es: la consulta moría con 22P02 y no cerraba ni
// una sesión. Lo mismo le pasaba a cualquier llamante que pasara algo que no fuera
// un uuid —y `handleMeSessionRevokeOthers` pasaba un hash de 64 hex—, así que la
// revocación masiva NUNCA funcionó. El `nullif` de abajo da el significado que
// hacía falta, e `IS DISTINCT FROM` lo compara bien contra NULL, donde `<>` habría
// dado NULL y tampoco habría cerrado nada. Un `$2` que no sea ni vacío ni un uuid
// sigue fallando en voz alta, que es lo correcto: es un error de quien llama.
func (s *Store) RevokeOtherSessions(ctx context.Context, q pg.Querier, userID, keepSessionID string) (int, error) {
	if q == nil {
		q = s.DB()
	}
	rows, err := q.Query(ctx, `UPDATE sessions SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
		  AND id IS DISTINCT FROM nullif($2::text,'')::uuid
		RETURNING id`, userID, keepSessionID)
	if err != nil {
		return 0, errf("RevokeOtherSessions", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, errf("RevokeOtherSessions.scan", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, errf("RevokeOtherSessions.iter", err)
	}
	// ponytail: un UPDATE por sesión revocada; las sesiones vivas por usuario
	// se cuentan con los dedos, no compensa un CTE conjunto.
	for _, id := range ids {
		if err := s.RevokeRefreshTokensForSession(ctx, q, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// RevokeAllUserSessions revoca TODAS las sesiones SSO vigentes del usuario y, en
// cascada, todos sus refresh tokens. Es el cierre forzado por un admin (cuenta
// comprometida o suspensión): corta el acceso ya emitido sin esperar a que
// caduque. Devuelve cuántas sesiones cerró. Ejecutar en transacción para que
// sesiones y refresh caigan juntos.
// CountLiveSessions cuenta las sesiones vivas de un usuario.
//
// Existe para poder DECIR cuántas se van a cerrar. Suspender ya arrastra las
// sesiones dentro de SetUserStatus (revokeIfInactive), así que un
// RevokeAllUserSessions posterior encuentra cero y el «se cerraron N sesiones»
// que la consola enseña salía siempre en cero: el dato hay que tomarlo ANTES, y
// en la misma transacción que la baja para que no cambie en medio.
func (s *Store) CountLiveSessions(ctx context.Context, q pg.Querier, userID string) (int, error) {
	if q == nil {
		q = s.DB()
	}
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()`, userID).Scan(&n)
	return n, errf("CountLiveSessions", err)
}

func (s *Store) RevokeAllUserSessions(ctx context.Context, q pg.Querier, userID string) (int, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `UPDATE sessions SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return 0, errf("RevokeAllUserSessions", err)
	}
	// Todos los refresh del usuario caen en un único UPDATE (no por-sesión: aquí
	// no se conserva ninguna, así que no hace falta ligarlos a session_id).
	if err := s.RevokeRefreshTokensForUser(ctx, q, userID); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// --- Refresh tokens (rotación) ---

// InsertRefreshToken persiste un refresh token (solo el hash).
func (s *Store) InsertRefreshToken(ctx context.Context, q pg.Querier, rt domain.RefreshToken) (string, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO refresh_tokens
		(token_hash, client_id, user_id, org_id, session_id, scope, auth_time, expires_at)
		VALUES ($1,$2,$3,nullif($4,'')::uuid,nullif($5,'')::uuid,$6,$7,$8)
		RETURNING id`
	var id string
	err := q.QueryRow(ctx, sql, rt.TokenHash, rt.ClientID, rt.UserID, rt.OrgID,
		rt.SessionID, rt.Scope, rt.AuthTime, rt.ExpiresAt).Scan(&id)
	return id, errf("InsertRefreshToken", err)
}

// GetRefreshToken recupera un refresh token por hash.
func (s *Store) GetRefreshToken(ctx context.Context, q pg.Querier, tokenHash string) (domain.RefreshToken, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `SELECT id, token_hash, client_id, user_id, coalesce(org_id::text,''),
		coalesce(session_id::text,''), scope, auth_time, coalesce(rotated_to::text,''),
		expires_at, revoked_at
		FROM refresh_tokens WHERE token_hash = $1 FOR UPDATE`
	var rt domain.RefreshToken
	err := q.QueryRow(ctx, sql, tokenHash).Scan(&rt.ID, &rt.TokenHash,
		&rt.ClientID, &rt.UserID, &rt.OrgID, &rt.SessionID, &rt.Scope,
		&rt.AuthTime, &rt.RotatedTo, &rt.ExpiresAt, &rt.RevokedAt)
	return rt, errf("GetRefreshToken", err)
}

// RotateRefreshToken marca el antiguo como revocado y enlazado al sucesor.
func (s *Store) RotateRefreshToken(ctx context.Context, q pg.Querier, oldID, newID string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `UPDATE refresh_tokens
		SET revoked_at = now(), rotated_to = $2 WHERE id = $1`, oldID, newID)
	return errf("RotateRefreshToken", err)
}

// RevokeRefreshChain revoca toda la familia de un refresh token (reuso detectado):
// revoca el token y todos los descendientes alcanzables por rotated_to.
func (s *Store) RevokeRefreshChain(ctx context.Context, q pg.Querier, rootID string) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `WITH RECURSIVE chain AS (
			SELECT id, rotated_to FROM refresh_tokens WHERE id = $1
			UNION ALL
			SELECT rt.id, rt.rotated_to FROM refresh_tokens rt
				JOIN chain c ON rt.id = c.rotated_to
		)
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE id IN (SELECT id FROM chain) AND revoked_at IS NULL`
	_, err := q.Exec(ctx, sql, rootID)
	return errf("RevokeRefreshChain", err)
}

// RevokeRefreshTokensForUser revoca todos los refresh tokens de un usuario.
func (s *Store) RevokeRefreshTokensForUser(ctx context.Context, q pg.Querier, userID string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return errf("RevokeRefreshTokensForUser", err)
}

// RevokeRefreshTokensForUserInOrg revoca los refresh tokens de un usuario ligados a
// UNA organización concreta. Lo usa la baja de membresía (RemoveMembership): sin
// esto, a quien se echa de una org le quedaba un refresh válido con el `org_id` de
// esa org, y seguía mintando access tokens con su claim `tenant` indefinidamente.
//
// Acotado a la org a propósito: un usuario puede pertenecer a varias, y salir de una
// no debe cerrarle la sesión en las demás. Los tokens SIN org (org_id NULL, p. ej. de
// un cliente que aún no eligió organización) no se tocan: no conceden acceso a datos
// de la org.
func (s *Store) RevokeRefreshTokensForUserInOrg(ctx context.Context, q pg.Querier, userID, orgID string) error {
	if q == nil {
		q = s.DB()
	}
	if userID == "" || orgID == "" {
		return nil
	}
	_, err := q.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND org_id = $2::uuid AND revoked_at IS NULL`, userID, orgID)
	return errf("RevokeRefreshTokensForUserInOrg", err)
}

// SetSessionsOrg cambia la organización activa de TODAS las sesiones SSO vivas
// del usuario y devuelve cuántas cambió.
//
// La organización activa se decidía al abrir la sesión y era la membresía más
// antigua (`mems[0]` en login.go y webauthn_login.go), sin forma de cambiarla:
// quien pertenece a dos organizaciones solo podía administrar la más vieja, y
// si en ella era `member`, la consola le daba 403 para siempre aunque fuera
// `owner` de la otra. Esto es la palanca que faltaba.
//
// Se cambian TODAS las sesiones vivas y no solo la del navegador que lo pide:
// «mi organización activa» es una propiedad de la persona, no de una pestaña, y
// dejar cada dispositivo en una organización distinta es exactamente la clase de
// estado que nadie recuerda haber elegido. Quien llama tiene que haber
// comprobado ANTES que el usuario es miembro de esa organización: aquí no hay
// contexto para hacerlo.
func (s *Store) SetSessionsOrg(ctx context.Context, q pg.Querier, userID, orgID string) (int, error) {
	if q == nil {
		q = s.DB()
	}
	if userID == "" || orgID == "" {
		return 0, nil
	}
	tag, err := q.Exec(ctx, `UPDATE sessions SET org_id = $2::uuid
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()`, userID, orgID)
	if err != nil {
		return 0, errf("SetSessionsOrg", err)
	}
	return int(tag.RowsAffected()), nil
}

// RevokeRefreshTokensForSession revoca todos los refresh tokens ligados a una
// sesión SSO. Se usa en el logout para invalidar en cascada los refresh derivados
// de una sesión que se cierra o se compromete.
func (s *Store) RevokeRefreshTokensForSession(ctx context.Context, q pg.Querier, sessionID string) error {
	if q == nil {
		q = s.DB()
	}
	if sessionID == "" {
		return nil
	}
	_, err := q.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now()
		WHERE session_id = nullif($1,'')::uuid AND revoked_at IS NULL`, sessionID)
	return errf("RevokeRefreshTokensForSession", err)
}

// CleanupExpired borra códigos, sesiones, refresh tokens caducados y buckets
// de rate-limit ociosos (cron).
func (s *Store) CleanupExpired(ctx context.Context, now time.Time) error {
	_, err := s.DB().Exec(ctx, `DELETE FROM auth_codes WHERE expires_at < $1`, now)
	if err != nil {
		return errf("CleanupExpired.codes", err)
	}
	_, err = s.DB().Exec(ctx, `DELETE FROM webauthn_challenges WHERE expires_at < $1`, now)
	if err != nil {
		return errf("CleanupExpired.challenges", err)
	}
	_, err = s.DB().Exec(ctx, `DELETE FROM mfa_continuations WHERE expires_at < $1`, now)
	if err != nil {
		return errf("CleanupExpired.mfa", err)
	}
	// Tokens de reseteo caducados (vida corta). Usa password_reset_tokens_expires_idx.
	_, err = s.DB().Exec(ctx, `DELETE FROM password_reset_tokens WHERE expires_at < $1`, now)
	if err != nil {
		return errf("CleanupExpired.reset", err)
	}
	// Tokens de verificación de email caducados. Usa email_verification_tokens_expires_idx.
	_, err = s.DB().Exec(ctx, `DELETE FROM email_verification_tokens WHERE expires_at < $1`, now)
	if err != nil {
		return errf("CleanupExpired.emailverify", err)
	}
	// Altas por SMS que nadie terminó. Usa signup_phone_codes_expires_idx.
	_, err = s.DB().Exec(ctx, `DELETE FROM signup_phone_codes WHERE expires_at < $1`, now)
	if err != nil {
		return errf("CleanupExpired.signupcodes", err)
	}
	// Sesiones caducadas hace >7 días. El margen deja que el logout/cascada de
	// revocación las consulte justo tras expirar. Usa sessions_expires_idx.
	//
	// `$1::timestamptz` y no `$1` a secas: en `$1 - interval '7 days'` PostgreSQL
	// no tiene de dónde deducir el tipo del parámetro y lo resuelve como interval,
	// con lo que la comparación queda `timestamptz < interval` y la consulta muere
	// con «operator does not exist» (SQLSTATE 42883). Y como esta función va en
	// secuencia, el fallo se llevaba por delante también la purga de refresh
	// tokens y todo lo que viene detrás: en producción se veía una línea de WARN
	// cada diez minutos y las dos tablas que más crecen no se limpiaban nunca.
	_, err = s.DB().Exec(ctx, `DELETE FROM sessions WHERE expires_at < $1::timestamptz - interval '7 days'`, now)
	if err != nil {
		return errf("CleanupExpired.sessions", err)
	}
	// Refresh tokens caducados hace >30 días. refresh_tokens rota en cada refresh
	// (fila nueva por rotación) => es la tabla que más crece; el margen de 30 días
	// conserva la ventana de detección de replay (rotated_to) tras la expiración.
	// Solo por expires_at para usar refresh_tokens_expires_idx (Index Scan, no
	// Seq Scan): los tokens revocados se limpian igual al llegar su expires_at.
	// ponytail: retención fija de 30 días; añadir un índice parcial sobre
	// revoked_at si a escala interesa purgar antes los revocados de vida larga.
	_, err = s.DB().Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at < $1::timestamptz - interval '30 days'`, now)
	if err != nil {
		return errf("CleanupExpired.refresh", err)
	}
	// Estados de flujo federado (RP) caducados: vida cortísima (minutos), un solo
	// uso. Usa federated_auth_states_expires_idx (Index Scan).
	_, err = s.DB().Exec(ctx, `DELETE FROM federated_auth_states WHERE expires_at < $1`, now)
	if err != nil {
		return errf("CleanupExpired.federated", err)
	}
	// Firmas de captchas ALTCHA resueltos (mig 0110) ya caducadas: el reto solo
	// vive diez minutos, así que a esa edad la firma ya no puede repetirse.
	_, err = s.DB().Exec(ctx, `DELETE FROM captcha_used WHERE expires_at < $1`, now)
	if err != nil {
		return errf("CleanupExpired.captcha", err)
	}
	// Buckets del limitador compartido (login/por-cuenta) ociosos >1h: a esa
	// edad están recargados del todo, borrarlos no cambia el veredicto. Acota la
	// tabla platform_rate_limits (una fila por IP/email visto).
	if _, err = pg.PurgeRateLimits(ctx, s.DB(), time.Hour); err != nil {
		return errf("CleanupExpired.ratelimits", err)
	}
	// Avisos y errores capturados (mig 0049) de más de siete días. Es
	// diagnóstico reciente, no un archivo: lo que se mira aquí es qué está
	// fallando ahora, y el histórico de verdad va al SIEM del cliente. Sin poda
	// crecería igual que crecían las sesiones antes de tener la suya, y encima
	// más rápido: una tormenta de errores escribe miles de filas por minuto.
	if _, err = s.PurgeServiceLogs(ctx, now.Add(-7*24*time.Hour)); err != nil {
		return errf("CleanupExpired.servicelogs", err)
	}
	if s.externalAccess.Apply != nil {
		// Only acknowledged activations expire. A prolonged transport outage
		// must not erase a committed activation that still needs delivery.
		if _, err = events.Purge(ctx, s.DB(), now.Add(-7*24*time.Hour), time.Time{}); err != nil {
			return errf("CleanupExpired.matrixaccess", err)
		}
	}
	return nil
}
