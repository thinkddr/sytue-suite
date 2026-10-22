// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Invitation es el destino materializado de una invitación al aceptarla: a qué
// organización y con qué rol se añade al invitado. El email viaja para resolver
// (o crear) la cuenta. No incluye el token en claro (solo su hash vive en BD).
type Invitation struct {
	OrgID string
	Email string
	Role  string
	// MailboxLocal es la dirección de correo que eligió quien invitó, para no
	// aplicar el patrón nombre.apellidos (vacío ⇒ patrón por defecto). Viaja en la
	// fila y no en el enlace por lo mismo que el rol: al aceptar no se confía en
	// nada que llegue del navegador.
	MailboxLocal string
	// Name es el nombre que dio quien invitó (vacío ⇒ la parte local del correo).
	// Solo se usa si la cuenta se crea al aceptar: una que ya existía conserva
	// el suyo.
	Name string
}

// ResolveOrgMemberByEmail resuelve un correo SOLO dentro de la organización
// indicada. Lo usa la integración de Cloud para evitar invitar de nuevo a quien
// ya pertenece a la suite y devolver su `sub` estable sin enumeración cross-org.
func (s *Store) ResolveOrgMemberByEmail(ctx context.Context, orgID, email string) (userID, role string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	err = s.DB().QueryRow(ctx, `SELECT u.id::text, m.role
		FROM users u JOIN memberships m ON m.user_id=u.id
		WHERE m.org_id=$1 AND lower(u.email)=$2 AND u.status='active'`,
		orgID, email).Scan(&userID, &role)
	return userID, role, errf("ResolveOrgMemberByEmail", err)
}

// ResolverDireccionesDeOrg traduce direcciones a la persona ACTIVA de la organización
// que la usa hoy: su correo de entrar VERIFICADO o puesto por el IdP de la organización
// (SCIM, `external_id`; esas cuentas nacen sin verificar); uno sin verificar lo escribe
// cualquiera, y bastaría para volver ambigua la dirección de otro o una de sus
// direcciones de buzón que no se está retirando. Las que no son de nadie de ella, o de dos a la vez, no salen.
func (s *Store) ResolverDireccionesDeOrg(ctx context.Context, orgID string, emails []string) (map[string]string, error) {
	if len(emails) == 0 {
		return map[string]string{}, nil
	}
	// Dos ramas y no un OR: cada una usa su índice (el correo de entrar y la dirección).
	rows, err := s.DB().Query(ctx, `
		SELECT d.email, u.id::text
		FROM unnest($2::text[]) AS d(email)
		JOIN users u ON lower(u.email) = d.email AND u.status = 'active' AND (u.email_verified OR u.external_id IS NOT NULL)
		JOIN memberships m ON m.user_id = u.id AND m.org_id = $1
		UNION
		SELECT d.email, u.id::text
		FROM unnest($2::text[]) AS d(email)
		JOIN org_mailboxes b ON lower(b.address) = d.email AND b.org_id = $1 AND b.removed_at IS NULL
		JOIN users u ON u.id = b.user_id AND u.status = 'active'
		JOIN memberships m ON m.user_id = u.id AND m.org_id = $1`, orgID, emails)
	if err != nil {
		return nil, errf("ResolverDireccionesDeOrg", err)
	}
	defer rows.Close()
	return sinAmbiguas(rows, "ResolverDireccionesDeOrg")
}

// DuenosGlobales son, por dirección, las cuentas activas que la usan hoy en el directorio
// global (las cuentas son globales, ADR 0040): por su correo de entrar VERIFICADO (aquí
// sin la excepción de SCIM de ResolverDireccionesDeOrg: el IdP de UNA organización no
// puede afirmar direcciones de otra) o por
// un buzón que no se está retirando, de cualquier organización. `Valido` dice si esa
// forma de tenerla cuenta: el buzón, solo de quien sigue siendo miembro de una
// organización activa. Los buzones que no cuentan siguen saliendo, para ver que una
// dirección es de dos a la vez; un correo de entrar sin verificar no sale ni para eso:
// cualquiera puede escribirlo al darse de alta y, si contara, bastaría para dejar sin
// avisos al dueño de verdad.
func (s *Store) DuenosGlobales(ctx context.Context, emails []string) (map[string][]DuenoGlobal, error) {
	out := map[string][]DuenoGlobal{}
	if len(emails) == 0 {
		return out, nil
	}
	rows, err := s.DB().Query(ctx, `
		SELECT d.email, u.id::text, true
		FROM unnest($1::text[]) AS d(email)
		JOIN users u ON lower(u.email) = d.email AND u.status = 'active' AND u.email_verified
		UNION
		SELECT d.email, u.id::text,
		       EXISTS (SELECT 1 FROM memberships m JOIN organizations o ON o.id = m.org_id
		               WHERE m.user_id = u.id AND m.org_id = b.org_id AND o.status = 'active')
		FROM unnest($1::text[]) AS d(email)
		JOIN org_mailboxes b ON lower(b.address) = d.email AND b.removed_at IS NULL
		JOIN users u ON u.id = b.user_id AND u.status = 'active'`, emails)
	if err != nil {
		return nil, errf("DuenosGlobales", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e string
		var d DuenoGlobal
		if err := rows.Scan(&e, &d.Sub, &d.Valido); err != nil {
			return nil, errf("DuenosGlobales.scan", err)
		}
		out[e] = append(out[e], d)
	}
	return out, errf("DuenosGlobales.rows", rows.Err())
}

// DuenoGlobal es una cuenta que usa una dirección (DuenosGlobales).
type DuenoGlobal struct {
	Sub    string
	Valido bool
}

// Unico es el `sub` de la dirección si es de UNA sola cuenta y esa forma de tenerla
// cuenta; "" si es de nadie, de varias, o no cuenta.
func Unico(duenos []DuenoGlobal) string {
	sub, valido := "", false
	for _, d := range duenos {
		if sub != "" && d.Sub != sub {
			return ""
		}
		sub = d.Sub
		valido = valido || d.Valido
	}
	if !valido {
		return ""
	}
	return sub
}

// MiembrosActivos son, de `subs`, las cuentas activas que son miembros de la
// organización: quien puede abrir hoy algo de ella con su propia sesión.
func (s *Store) MiembrosActivos(ctx context.Context, orgID string, subs []string) ([]string, error) {
	if len(subs) == 0 {
		return nil, nil
	}
	rows, err := s.DB().Query(ctx, `
		SELECT u.id::text FROM users u
		JOIN memberships m ON m.user_id = u.id AND m.org_id = $1
		WHERE u.id = ANY ($2::uuid[]) AND u.status = 'active'
		ORDER BY 1`, orgID, subs)
	if err != nil {
		return nil, errf("MiembrosActivos", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, errf("MiembrosActivos.scan", err)
		}
		out = append(out, id)
	}
	return out, errf("MiembrosActivos.rows", rows.Err())
}

// sinAmbiguas lee (dirección, sub) y descarta la dirección que sale con dos personas.
func sinAmbiguas(rows pgx.Rows, op string) (map[string]string, error) {
	out := map[string]string{}
	ambiguas := map[string]bool{}
	for rows.Next() {
		var email, id string
		if err := rows.Scan(&email, &id); err != nil {
			return nil, errf(op+".scan", err)
		}
		if previo, ya := out[email]; ya && previo != id {
			ambiguas[email] = true
		}
		out[email] = id
	}
	for e := range ambiguas {
		delete(out, e)
	}
	return out, errf(op+".rows", rows.Err())
}

// ResolveOrgMemberByID hace el camino de vuelta: del identificador al correo y
// al nombre, dentro de la MISMA organización.
//
// Sirve para pintar «compartido con quien sea» sin enseñar un identificador
// opaco. Va acotado a la org igual que la ida: si esa persona no está en ella, no
// existe para quien pregunta.
func (s *Store) ResolveOrgMemberByID(ctx context.Context, orgID, userID string) (email, name string, err error) {
	err = s.DB().QueryRow(ctx, `SELECT u.email, u.name
		FROM users u JOIN memberships m ON m.user_id=u.id
		WHERE m.org_id=$1 AND u.id::text=$2 AND u.status='active'`,
		orgID, strings.TrimSpace(userID)).Scan(&email, &name)
	return email, name, errf("ResolveOrgMemberByID", err)
}

// RevokeInvitationByEmail borra cualquier invitación pendiente de ese correo en
// la organización. Es idempotente: false significa que ya no existía.
func (s *Store) RevokeInvitationByEmail(ctx context.Context, orgID, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	tag, err := s.DB().Exec(ctx, `DELETE FROM org_invitations
		WHERE org_id=$1 AND lower(email)=$2`, orgID, email)
	if err != nil {
		return false, errf("RevokeInvitationByEmail", err)
	}
	return tag.RowsAffected() > 0, nil
}

// InsertInvitation persiste el hash de un token de invitación con su destino
// (org + rol + email). Antes purga las invitaciones previas de la MISMA (org,
// email): al reinvitar, los enlaces anteriores dejan de valer (solo el último).
// Debe ejecutarse en transacción (purgar + insertar) para que sea atómico. Mismo
// patrón que InsertResetToken.
// `mailboxLocal` es la dirección de correo elegida para el invitado (vacía ⇒
// patrón por defecto); se aplica al aceptar, no al invitar, porque hasta entonces
// no hay usuario al que asignársela. `name` es el nombre de la persona (mig
// 0113), por lo mismo: se le pone a la cuenta si la crea al aceptar.
func (s *Store) InsertInvitation(ctx context.Context, q pg.Querier, tokenHash, orgID, email, role, inviterID, mailboxLocal, name string, expires time.Time) error {
	if q == nil {
		q = s.DB()
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := q.Exec(ctx,
		`DELETE FROM org_invitations WHERE org_id = $1 AND lower(email) = $2`,
		orgID, email); err != nil {
		return errf("InsertInvitation.purge", err)
	}
	const sql = `INSERT INTO org_invitations
		(token_hash, org_id, email, role, inviter_id, expires_at, mailbox_local, invitee_name)
		VALUES ($1, $2, $3, $4, nullif($5,'')::uuid, $6, $7, $8)`
	_, err := q.Exec(ctx, sql, tokenHash, orgID, email, role, inviterID, expires, mailboxLocal,
		domain.SanitizeDisplayName(name))
	return errf("InsertInvitation", err)
}

// ConsumeInvitation recupera y BORRA una invitación válida (atómico, un-solo-uso,
// anti-replay), devolviendo su destino (org + rol + email). Una invitación
// caducada, ya aceptada o inexistente ⇒ ErrNotFound (indistinguibles para el
// llamante). Mismo patrón que ConsumeResetToken.
func (s *Store) ConsumeInvitation(ctx context.Context, q pg.Querier, tokenHash string, now time.Time) (Invitation, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `DELETE FROM org_invitations
		WHERE token_hash = $1 AND expires_at > $2
		RETURNING org_id::text, email, role, mailbox_local, invitee_name`
	var inv Invitation
	err := q.QueryRow(ctx, sql, tokenHash, now).Scan(&inv.OrgID, &inv.Email, &inv.Role, &inv.MailboxLocal, &inv.Name)
	return inv, errf("ConsumeInvitation", err)
}

// InvitationPending dice si un enlace de invitación SIGUE VALIENDO: existe y no
// ha caducado. No la consume ni la bloquea.
//
// Existe porque revocar borra la fila y la pantalla de aceptación solo miraba la
// FIRMA del token: el enlace revocado seguía abriendo el formulario, y quien lo
// hubiera recibido escribía una contraseña para que el envío le dijera que no,
// con la duda razonable de si la cuenta se había creado o no.
func (s *Store) InvitationPending(ctx context.Context, tokenHash string, now time.Time) (bool, error) {
	var existe bool
	err := s.DB().QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM org_invitations WHERE token_hash = $1 AND expires_at > $2)`,
		tokenHash, now).Scan(&existe)
	return existe, errf("InvitationPending", err)
}

// CountOrgAdmins cuenta el conjunto de administración (owner|admin) de una
// organización para los guards anti-lockout, con la misma regla que
// CountOrgOwners: las cuentas activas más `targetID`, la cuenta sobre la que se
// actúa (ver allí).
func (s *Store) CountOrgAdmins(ctx context.Context, q pg.Querier, orgID, targetID string) (int, error) {
	return s.contarAdministracion(ctx, q, orgID, targetID, `m.role IN ('owner','admin')`)
}

// CountOrgOwners cuenta los OWNERS de una organización. Sirve el guard anti-lockout
// «≥1 owner»: una org SIEMPRE debe conservar al menos un owner, así que no se puede
// degradar/quitar al último (a diferencia de CountOrgAdmins, que protege el conjunto
// más amplio owner|admin).
//
// Cuenta las propietarias ACTIVAS más `targetID`, la cuenta sobre la que se actúa,
// esté como esté: así `n <= 1` sigue queriendo decir «no queda ninguna otra
// activa». Una propietaria dada de baja conserva su membresía durante la ventana de
// restauración, y contarla dejaba quitar a la última de verdad; no contar a la
// propia cuenta, al revés, impedía retirar a una propietaria suspendida o borrada
// habiendo otra activa. `targetID` vacío: solo las activas.
func (s *Store) CountOrgOwners(ctx context.Context, q pg.Querier, orgID, targetID string) (int, error) {
	return s.contarAdministracion(ctx, q, orgID, targetID, `m.role = 'owner'`)
}

func (s *Store) contarAdministracion(ctx context.Context, q pg.Querier, orgID, targetID, rol string) (int, error) {
	if q == nil {
		q = s.DB()
	}
	// El bloqueo de la organización es el mismo que toma «Borrar mi cuenta»
	// (BorrarCuentaPropia): sin él, una baja de un admin y la baja propia de la otra
	// propietaria podían cruzarse y dejarla sin nadie. NO KEY UPDATE y no UPDATE: no
	// choca con el FOR KEY SHARE que toma cada fila nueva que referencia a la
	// organización (sesiones, refresh), así que no frena los inicios de sesión.
	// Fuera de una transacción no hace nada, y no molesta.
	if _, err := q.Exec(ctx, `SELECT 1 FROM organizations WHERE id = $1 FOR NO KEY UPDATE`, orgID); err != nil {
		return 0, errf("contarAdministracion.lock", err)
	}
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM memberships m JOIN users u ON u.id = m.user_id
		  WHERE m.org_id = $1 AND `+rol+` AND (u.status = 'active' OR u.id::text = $2)`,
		orgID, targetID).Scan(&n)
	return n, errf("contarAdministracion", err)
}

// ClaimPendingInvitations materializa las invitaciones VIVAS del correo dado:
// crea la membresía de cada una y borra la fila.
//
// Existe porque una invitación puede quedarse esperando para siempre. El enlace
// viaja por correo, y el correo se pierde —filtros, buzón mal escrito, un MSA
// caído—: entonces la persona acaba entrando por su cuenta, no pertenece a
// ninguna organización y la suite le recibe con el asistente de alta
// preguntándole «¿para quién es Sytue?», cuando alguien ya la había dado de alta
// en su empresa. Lo que se le pide es que configure una organización que ya
// existe y en la que ya tiene sitio.
//
// EXIGE el correo verificado, y ese es el punto: aceptar por el enlace prueba
// control del buzón, y esta vía tiene que probar lo mismo o sería una forma de
// entrar en una organización ajena registrándose con la dirección adecuada. Con
// el correo sin verificar no se materializa nada y la invitación sigue esperando
// a su enlace.
//
// Devuelve las organizaciones en las que ha quedado dentro.
func (s *Store) ClaimPendingInvitations(ctx context.Context, q pg.Querier, userID, email string, now time.Time) ([]Invitation, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `DELETE FROM org_invitations
		WHERE lower(email) = lower($1) AND expires_at > $2
		RETURNING org_id, email, role, coalesce(mailbox_local, '')`
	rows, err := q.Query(ctx, sql, email, now)
	if err != nil {
		return nil, errf("ClaimPendingInvitations", err)
	}
	var pendientes []Invitation
	for rows.Next() {
		var inv Invitation
		if err := rows.Scan(&inv.OrgID, &inv.Email, &inv.Role, &inv.MailboxLocal); err != nil {
			rows.Close()
			return nil, errf("ClaimPendingInvitations.scan", err)
		}
		pendientes = append(pendientes, inv)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, errf("ClaimPendingInvitations.iter", err)
	}

	for _, inv := range pendientes {
		// UpsertMembership y no INSERT: si ya era miembro por otra vía, la
		// invitación simplemente sobraba y no puede fallar por eso.
		if err := s.UpsertMembership(ctx, q, domain.Membership{
			OrgID: inv.OrgID, UserID: userID, Role: domain.Role(inv.Role),
		}); err != nil {
			return nil, errf("ClaimPendingInvitations.membresia", err)
		}
		// El buzón de la organización, con la dirección que eligiera quien invitó.
		// Su fallo no anula el alta: sin dominio configurado no hay dirección que
		// dar, y quedarse fuera de la organización por eso sería peor.
		if _, err := s.EnsureOrgMailbox(ctx, q, inv.OrgID, userID, inv.MailboxLocal); err != nil {
			continue
		}
	}
	return pendientes, nil
}
