// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// --- «Borrar mi cuenta» (mig 0098, ADR 0054) ---------------------------------
//
// Tres momentos, cada uno con su función:
//
//  1. BorrarCuentaPropia, al confirmar: la cuenta queda dada de baja —con la
//     ventana de 30 días de la 0092— y se borra DE VERDAD, en la misma
//     transacción, todo lo que sirve para entrar: sesiones, tokens, contraseña,
//     passkeys, segundo factor, contraseñas de aplicación, dispositivos, enlaces
//     con IdP externos y la foto.
//  2. MarcarDatosPersonalesBorrados, cuando el barrido ha borrado lo que los
//     otros servicios guardan de esa persona (merge.DatosDeLaPersona).
//  3. PurgarCuentaPropia, vencida la ventana: se anonimiza la ficha y se quitan
//     las membresías. Desde ahí la baja ya no se puede deshacer.

// ErrUnicoAdministrador es una baja que dejaría alguna organización sin nadie
// que la administre.
var ErrUnicoAdministrador = errors.New("la cuenta es la única que administra una organización")

// OrgsSinOtroAdministrador devuelve el nombre de cada organización (de verdad,
// no la cuenta personal) que se quedaría sin administración si esta persona se
// fuera: es su propietaria y no hay otra activa, o la administra y no hay nadie
// activo más que sea propietario o administrador.
//
// La cuenta personal no cuenta: es de esta persona sola y se va con ella.
func (s *Store) OrgsSinOtroAdministrador(ctx context.Context, q pg.Querier, userID string) ([]string, error) {
	if q == nil {
		q = s.DB()
	}
	rows, err := q.Query(ctx, `
		SELECT o.name FROM memberships m
		JOIN organizations o ON o.id = m.org_id
		WHERE m.user_id = $1 AND m.role IN ('owner', 'admin') AND o.kind <> 'personal'
		  AND NOT EXISTS (
		      SELECT 1 FROM memberships otra
		      JOIN users u ON u.id = otra.user_id
		      WHERE otra.org_id = m.org_id AND otra.user_id <> $1 AND u.status = 'active'
		        AND (otra.role = 'owner' OR (m.role = 'admin' AND otra.role = 'admin')))
		ORDER BY o.name`, userID)
	if err != nil {
		return nil, errf("OrgsSinOtroAdministrador", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, errf("OrgsSinOtroAdministrador.scan", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, errf("OrgsSinOtroAdministrador.rows", err)
	}
	// La administración de la plataforma tampoco se puede quedar sin nadie.
	var ultimaDePlataforma bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM platform_admins WHERE user_id = $1)
		   AND NOT EXISTS (SELECT 1 FROM platform_admins p JOIN users u ON u.id = p.user_id
		                   WHERE p.user_id <> $1 AND u.status = 'active')`, userID).Scan(&ultimaDePlataforma); err != nil {
		return nil, errf("OrgsSinOtroAdministrador.plataforma", err)
	}
	if ultimaDePlataforma {
		out = append(out, NombrePlataforma)
	}
	return out, nil
}

// NombrePlataforma es como se nombra la administración de la plataforma cuando es
// ella la que se quedaría sin nadie.
const NombrePlataforma = "Sytue One (plataforma)"

// credencialesSQL borra todo lo que sirve para entrar o actuar como la cuenta. Se
// ejecuta al confirmar y OTRA VEZ en la purga: un access token vivo en los
// minutos que siguen a la baja podía crear una fila nueva antes de que
// requireAccessToken empezara a mirar el estado, y la purga no la dejaría atrás.
var credencialesSQL = []string{
	`DELETE FROM auth_codes WHERE user_id = $1`,
	`DELETE FROM refresh_tokens WHERE user_id = $1`,
	`DELETE FROM sessions WHERE user_id = $1`,
	`DELETE FROM webauthn_credentials WHERE user_id = $1`,
	`DELETE FROM webauthn_challenges WHERE user_id = $1`,
	`DELETE FROM totp_secrets WHERE user_id = $1`,
	`DELETE FROM mfa_continuations WHERE user_id = $1`,
	`DELETE FROM mfa_recovery_codes WHERE user_id = $1`,
	`DELETE FROM password_reset_tokens WHERE user_id = $1`,
	`DELETE FROM email_verification_tokens WHERE user_id = $1`,
	`DELETE FROM federated_identities WHERE user_id = $1`,
	`DELETE FROM devices WHERE user_id = $1`,
	`DELETE FROM app_passwords WHERE user_id = $1`,
	`DELETE FROM user_avatars WHERE user_id = $1`,
	`DELETE FROM role_activations WHERE user_id = $1`,
	`DELETE FROM platform_activations WHERE user_id = $1`,
	// Ser administradora de la plataforma no sobrevive a la baja: si se
	// restaurase, volvería sin ese poder y habría que concedérselo otra vez.
	`DELETE FROM platform_admins WHERE user_id = $1`,
	`UPDATE users SET password_hash = NULL, mfa_enabled = false WHERE id = $1`,
}

// OrgsSinOtroAdministradorBloqueando es OrgsSinOtroAdministrador dentro de la
// transacción del llamante (q no puede ser nil) y DESPUÉS de bloquear las
// organizaciones que la persona administra y la tabla de administración de la
// plataforma. Es lo que tiene que preguntar quien va a dejar sin acceso a alguien
// —su propia baja, o un operador que la bloquea o la elimina—: dos copropietarias
// que se van a la vez verían, cada una por su lado, que la otra sigue, y la
// organización se quedaría sin nadie. Con el bloqueo la segunda espera a la
// primera y, en READ COMMITTED, su consulta ya ve el cambio confirmado.
func (s *Store) OrgsSinOtroAdministradorBloqueando(ctx context.Context, q pg.Querier, userID string) ([]string, error) {
	if _, err := q.Exec(ctx, `
		SELECT 1 FROM organizations
		WHERE id IN (SELECT org_id FROM memberships WHERE user_id = $1 AND role IN ('owner', 'admin'))
		ORDER BY id FOR NO KEY UPDATE`, userID); err != nil {
		return nil, errf("OrgsSinOtroAdministradorBloqueando.lock", err)
	}
	// Y la administración de la plataforma, que no es una organización: dos
	// administradoras que se van a la vez no deben ver cada una a la otra activa.
	// La tabla es diminuta.
	if _, err := q.Exec(ctx, `SELECT 1 FROM platform_admins FOR UPDATE`); err != nil {
		return nil, errf("OrgsSinOtroAdministradorBloqueando.lock", err)
	}
	return s.OrgsSinOtroAdministrador(ctx, q, userID)
}

// BorrarCuentaPropia da de baja la cuenta a petición de su titular y borra sus
// credenciales. Va dentro de la transacción del llamante (q no puede ser nil).
//
// La comprobación de administración va AQUÍ, con bloqueo (ver
// OrgsSinOtroAdministradorBloqueando), y no en la página que la enseña.
//
// Devuelve false si la cuenta ya estaba dada de baja.
func (s *Store) BorrarCuentaPropia(ctx context.Context, q pg.Querier, userID string) (bool, error) {
	bloquean, err := s.OrgsSinOtroAdministradorBloqueando(ctx, q, userID)
	if err != nil {
		return false, err
	}
	if len(bloquean) > 0 {
		return false, ErrUnicoAdministrador
	}
	// closed_by = la propia cuenta es lo que marca la baja como propia (ver la mig 0098).
	cerrada, err := s.CloseUser(ctx, q, userID, userID, "self-service")
	if err != nil || !cerrada {
		return false, err
	}
	// Borrado, no revocación: RevokeAllUserSessions (dentro de CloseUser) ya dejó
	// todo sin validez; esto quita las filas. Los refresh van antes que las
	// sesiones solo por claridad: su session_id es ON DELETE SET NULL.
	for _, sql := range credencialesSQL {
		if _, err := q.Exec(ctx, sql, userID); err != nil {
			return false, errf("BorrarCuentaPropia", err)
		}
	}
	return true, nil
}

// CuentaPropiaPendiente es una baja propia a la que le falta algún paso.
type CuentaPropiaPendiente struct {
	ID       string
	ClosedAt time.Time
	// DatosBorrados dice si ya se borró lo que guardan los otros servicios.
	DatosBorrados bool
}

// CuentasPropiasPendientes lista las bajas propias sin terminar: las que aún
// tienen datos personales en otros servicios y se cerraron antes de `borrarDesde`,
// y las que ya cumplieron la ventana (`antes`) y esperan la purga.
//
// `borrarDesde` deja pasar la vida de un access token: los otros servicios
// validan el JWT por su cuenta, así que hasta que caduca alguien podría volver a
// registrar un push o un borrador después de haberlos borrado.
//
// En orden ALEATORIO y no por antigüedad: con un lote fijo, unas pocas cuentas
// cuyo borrado falla siempre se quedarían las primeras y taparían a las demás.
func (s *Store) CuentasPropiasPendientes(ctx context.Context, borrarDesde, antes time.Time, limite int) ([]CuentaPropiaPendiente, error) {
	rows, err := s.DB().Query(ctx, `
		SELECT id, closed_at, personal_data_erased_at IS NOT NULL
		FROM users
		WHERE status = 'deleted' AND closed_by = id AND purged_at IS NULL
		  AND ((personal_data_erased_at IS NULL AND closed_at < $1) OR closed_at < $2)
		ORDER BY random() LIMIT $3`, borrarDesde, antes, limite)
	if err != nil {
		return nil, errf("CuentasPropiasPendientes", err)
	}
	defer rows.Close()
	var out []CuentaPropiaPendiente
	for rows.Next() {
		var c CuentaPropiaPendiente
		if err := rows.Scan(&c.ID, &c.ClosedAt, &c.DatosBorrados); err != nil {
			return nil, errf("CuentasPropiasPendientes.scan", err)
		}
		out = append(out, c)
	}
	return out, errf("CuentasPropiasPendientes.rows", rows.Err())
}

// MarcarDatosPersonalesBorrados deja escrito que los otros servicios ya no
// guardan nada personal de esta cuenta.
func (s *Store) MarcarDatosPersonalesBorrados(ctx context.Context, userID string) error {
	_, err := s.DB().Exec(ctx, `UPDATE users SET personal_data_erased_at = now()
		WHERE id = $1 AND status = 'deleted' AND closed_by = id`, userID)
	return errf("MarcarDatosPersonalesBorrados", err)
}

// PurgarCuentaPropia anonimiza una baja propia cuya ventana venció (`antes`) y le
// quita las membresías. Devuelve nil si no había nada que purgar —otra réplica
// llegó antes, se restauró o la ventana aún no ha vencido— y, si purgó, los
// slugs de sus cuentas personales (vacío, no nil, si no tenía ninguna): su
// contenido es lo que queda por borrar a mano.
//
// No es un DELETE de la fila, por dos razones. Las filas que la referencian sin
// cascada (auditoría de plataforma, fusiones) harían fallar el borrado; y su
// buzón en `org_mailboxes` —que es de la organización— tiene que seguir
// ocupando la dirección: si se liberase, otra persona podría quedarse con
// ana@... y recibir lo que siguen mandando a la que se fue.
//
// La cuenta personal pierde su nombre, que es el correo de su titular, y se queda
// con su id. Su slug sigue siendo un resumen del correo (domain.PersonalSlug): es
// la clave de tenant en las diez bases y desaparece con el borrado manual de su
// contenido, que borra también la organización.
func (s *Store) PurgarCuentaPropia(ctx context.Context, userID string, antes time.Time) ([]string, error) {
	var personales []string
	err := s.InTx(ctx, func(q pg.Querier) error {
		tag, err := q.Exec(ctx, `
			UPDATE users SET
			    email = id::text || '@cuenta-borrada.invalid', email_verified = false,
			    username = NULL, name = '', given_name = '', family_name = '',
			    phone = NULL, recovery_email = NULL, external_id = NULL,
			    department = NULL, title = NULL, location = NULL,
			    password_hash = NULL, mfa_enabled = false, purged_at = now()
			WHERE id = $1 AND status = 'deleted' AND closed_by = id AND purged_at IS NULL
			  AND personal_data_erased_at IS NOT NULL AND closed_at < $2`, userID, antes)
		if err != nil {
			return errf("PurgarCuentaPropia", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		personales = []string{}
		rows, err := q.Query(ctx, `UPDATE organizations SET name = id::text
			WHERE kind = 'personal'
			  AND id IN (SELECT org_id FROM memberships WHERE user_id = $1 AND role = 'owner')
			RETURNING id::text`, userID)
		if err != nil {
			return errf("PurgarCuentaPropia.personal", err)
		}
		for rows.Next() {
			var slug string
			if err := rows.Scan(&slug); err != nil {
				rows.Close()
				return errf("PurgarCuentaPropia.personal.scan", err)
			}
			personales = append(personales, slug)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return errf("PurgarCuentaPropia.personal.rows", err)
		}
		for _, sql := range append(credencialesSQL,
			`DELETE FROM group_members WHERE user_id = $1`,
			`DELETE FROM user_custom_roles WHERE user_id = $1`,
			`DELETE FROM role_eligibilities WHERE user_id = $1`,
			`DELETE FROM onboarding_progress WHERE user_id = $1`,
			`DELETE FROM memberships WHERE user_id = $1`,
		) {
			if _, err := q.Exec(ctx, sql, userID); err != nil {
				return errf("PurgarCuentaPropia", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return personales, nil
}
