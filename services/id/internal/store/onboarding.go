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

// Persistencia del alta inicial (mig 0027). Dos ámbitos distintos:
//
//   - onboarding_progress: estado del asistente por USUARIO (la org aún no existe
//     en el primer paso). Toda consulta se acota por user_id: nadie lee ni escribe
//     el progreso de otro.
//   - org_profiles: dominio propio + datos fiscales de la ORGANIZACIÓN, acotados
//     por org_id como el resto de tablas por-tenant (§0.7).

// Onboarding es el estado del alta guiada de un usuario.
type Onboarding struct {
	UserID      string
	OrgID       string // "" si todavía no ha creado organización
	Mode        string
	Step        string
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const onboardingCols = `user_id::text, coalesce(org_id::text,''), mode, step, completed_at, created_at, updated_at`

// GetOnboarding devuelve el progreso del usuario. Sin fila ⇒ ErrNotFound (el
// usuario aún no ha empezado el alta).
func (s *Store) GetOnboarding(ctx context.Context, userID string) (Onboarding, error) {
	const sql = `SELECT ` + onboardingCols + ` FROM onboarding_progress WHERE user_id = $1`
	var o Onboarding
	err := s.DB().QueryRow(ctx, sql, userID).
		Scan(&o.UserID, &o.OrgID, &o.Mode, &o.Step, &o.CompletedAt, &o.CreatedAt, &o.UpdatedAt)
	return o, errf("GetOnboarding", err)
}

// UpsertOnboarding crea o actualiza el progreso. Convención de campos: un string
// VACÍO significa «no tocar» (el asistente guarda solo lo del paso en curso, y un
// PATCH parcial no puede borrar lo ya guardado). complete=true sella completed_at
// (idempotente: si ya estaba, conserva el sello original).
func (s *Store) UpsertOnboarding(ctx context.Context, q pg.Querier, userID, mode, step, orgID string, complete bool) (Onboarding, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO onboarding_progress (user_id, mode, step, org_id, completed_at)
		VALUES ($1, $2, $3, nullif($4,'')::uuid, CASE WHEN $5 THEN now() END)
		ON CONFLICT (user_id) DO UPDATE SET
			mode = CASE WHEN $2 = '' THEN onboarding_progress.mode ELSE $2 END,
			step = CASE WHEN $3 = '' THEN onboarding_progress.step ELSE $3 END,
			org_id = coalesce(nullif($4,'')::uuid, onboarding_progress.org_id),
			completed_at = CASE WHEN $5 THEN coalesce(onboarding_progress.completed_at, now())
			               ELSE onboarding_progress.completed_at END,
			updated_at = now()
		RETURNING ` + onboardingCols
	var o Onboarding
	err := q.QueryRow(ctx, sql, userID, mode, step, orgID, complete).
		Scan(&o.UserID, &o.OrgID, &o.Mode, &o.Step, &o.CompletedAt, &o.CreatedAt, &o.UpdatedAt)
	return o, errf("UpsertOnboarding", err)
}

// SlugTaken indica si el identificador ya lo usa una organización (incluidas las
// dadas de baja: el slug es la identidad del tenant en toda la suite y no se
// recicla). Es la comprobación REAL que respalda «disponible» en el asistente.
func (s *Store) SlugTaken(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := s.DB().QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM organizations WHERE slug = $1)`, slug).Scan(&exists)
	return exists, errf("SlugTaken", err)
}

// OrgProfile son el dominio propio PRINCIPAL y los datos fiscales de una
// organización.
//
// Los tres campos de dominio ya no se guardan aquí: desde la mig 0052 una
// organización tiene varios y viven en `org_mail_domains`. Se componen con el
// PRINCIPAL —el que estrenan las altas nuevas— para que todo lo que preguntaba
// «¿en qué dominio está el correo de esta organización?» siga teniendo respuesta
// sin saber que ahora hay más de uno.
type OrgProfile struct {
	OrgID            string
	Domain           string
	DomainToken      string
	DomainVerifiedAt *time.Time
	LegalName        string
	TaxID            string
	Address          string
	BillingEmail     string
}

// profileSQL sale de `organizations` y no de `org_profiles`: una organización
// puede tener dominio y no tener todavía fila de perfil —los datos fiscales son
// opcionales— y leyéndolo al revés su dominio no aparecería por ninguna parte.
// Y el dominio NO se puede leer solo por `is_primary`. Uno recién reclamado nace
// sin verificar y sin ser principal —lo asciende `MarkOrgMailDomainVerified`
// cuando aparece el TXT—, así que con esa condición la unión no encontraba nada
// entre reclamar y verificar: justo la ventana en la que el asistente de alta
// tiene que ENSEÑAR el registro que hay que publicar. Devolvía `_sytue.` y
// `sytue-verification=` a secas, sin dominio ni token, y el cliente se quedaba
// mirando un registro vacío que no podía crear.
//
// El orden dice cuál gana cuando hay varios: el principal; si ninguno lo es, uno
// verificado; y entre iguales, el último reclamado, que es el que la persona
// acaba de escribir en la pantalla.
const profileSQL = `
	SELECT o.id::text,
	       coalesce(d.domain, ''), coalesce(d.token, ''), d.verified_at,
	       coalesce(p.legal_name, ''), coalesce(p.tax_id, ''),
	       coalesce(p.address, ''), coalesce(p.billing_email, '')
	FROM organizations o
	LEFT JOIN org_profiles p ON p.org_id = o.id
	LEFT JOIN LATERAL (
	    SELECT domain, token, verified_at
	      FROM org_mail_domains
	     WHERE org_id = o.id
	     ORDER BY is_primary DESC, verified_at IS NULL, created_at DESC
	     LIMIT 1
	) d ON true
	WHERE o.id = $1`

func scanProfile(row pgx.Row) (OrgProfile, error) {
	var p OrgProfile
	err := row.Scan(&p.OrgID, &p.Domain, &p.DomainToken, &p.DomainVerifiedAt,
		&p.LegalName, &p.TaxID, &p.Address, &p.BillingEmail)
	return p, err
}

// GetOrgProfile devuelve el perfil de la org. Sin fila devuelve el perfil VACÍO
// (con OrgID) y nil: no tener datos fiscales ni dominio propio es el estado
// normal de una org recién creada, no un error que el llamante deba distinguir.
func (s *Store) GetOrgProfile(ctx context.Context, orgID string) (OrgProfile, error) {
	return s.orgProfile(ctx, nil, orgID)
}

// orgProfile lee el perfil con el Querier que se le dé, para poder componerlo
// dentro de la misma transacción que acaba de escribirlo.
func (s *Store) orgProfile(ctx context.Context, q pg.Querier, orgID string) (OrgProfile, error) {
	if q == nil {
		q = s.DB()
	}
	p, err := scanProfile(q.QueryRow(ctx, profileSQL, orgID))
	switch e := errf("GetOrgProfile", err); {
	case e == nil:
		return p, nil
	case errors.Is(e, domain.ErrNotFound):
		return OrgProfile{OrgID: orgID}, nil
	default:
		return OrgProfile{}, e
	}
}

// UpsertOrgBilling fija los datos fiscales/de facturación (todos opcionales: el
// alta no se bloquea por ellos) sin tocar el dominio ni su verificación.
//
// Escribe y RELEE en vez de devolver el RETURNING: el perfil ya no sale de una
// sola tabla —el dominio vive en org_mail_domains desde la mig 0052— y componer
// a mano lo que devuelve el INSERT sería la primera copia que se queda vieja.
func (s *Store) UpsertOrgBilling(ctx context.Context, q pg.Querier, orgID, legalName, taxID, address, billingEmail string) (OrgProfile, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO org_profiles (org_id, legal_name, tax_id, address, billing_email)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (org_id) DO UPDATE SET
			legal_name = $2, tax_id = $3, address = $4, billing_email = $5, updated_at = now()`
	if _, err := q.Exec(ctx, sql, orgID, legalName, taxID, address, billingEmail); err != nil {
		return OrgProfile{}, errf("UpsertOrgBilling", err)
	}
	return s.orgProfile(ctx, q, orgID)
}

// SetOrgDomain reclama EL dominio propio de la organización desde el asistente de
// alta, que solo conoce uno. Conserva token y sello si ya estaba reclamado
// (reintentar no reinicia nada ni invalida el registro que el cliente publicó) y
// se lleva por delante la reclamación anterior que siguiera sin verificar: en esa
// pantalla se teclea un dominio y se corrige la errata, y dejar cada intento
// fallido convertido en una fila obligaría a limpiarlos desde otra pantalla.
//
// Los dominios VERIFICADOS no se tocan aquí: quitar uno es una decisión aparte
// (DeleteOrgMailDomain), y hacerlo de rebote al teclear otro dejaría a la
// organización sin el dominio que ya usa su plantilla.
//
// Dominio ya verificado por otra org ⇒ ErrConflict.
func (s *Store) SetOrgDomain(ctx context.Context, q pg.Querier, orgID, host, token string) (OrgProfile, error) {
	var escrito OrgMailDomain
	escribir := func(q pg.Querier) error {
		if _, err := q.Exec(ctx, `
			DELETE FROM org_mail_domains
			WHERE org_id = $1 AND domain <> lower($2) AND verified_at IS NULL`,
			orgID, host); err != nil {
			return errf("SetOrgDomain.limpiar", err)
		}
		var err error
		escrito, err = s.AddOrgMailDomain(ctx, q, orgID, host, token)
		return err
	}
	if q == nil {
		if err := s.InTx(ctx, escribir); err != nil {
			return OrgProfile{}, err
		}
		return s.perfilConDominio(ctx, nil, orgID, escrito)
	}
	if err := escribir(q); err != nil {
		return OrgProfile{}, err
	}
	return s.perfilConDominio(ctx, q, orgID, escrito)
}

// perfilConDominio devuelve el perfil describiendo EL DOMINIO QUE SE ACABA DE
// ESCRIBIR, no el principal de la organización.
//
// La diferencia importa en el paso «cambiar de dominio» del asistente: una
// organización que ya tenía uno verificado lo conserva como principal, así que
// leer el perfil a secas devolvía ese —verificado— como respuesta a haber
// reclamado otro. La pantalla decía «verificado» de un dominio cuyo TXT nadie
// había publicado todavía, y el cliente no tenía por qué dudarlo.
func (s *Store) perfilConDominio(ctx context.Context, q pg.Querier, orgID string, d OrgMailDomain) (OrgProfile, error) {
	prof, err := s.orgProfile(ctx, q, orgID)
	if err != nil {
		return OrgProfile{}, err
	}
	prof.Domain, prof.DomainToken, prof.DomainVerifiedAt = d.Domain, d.Token, d.VerifiedAt
	return prof, nil
}

// MarkOrgDomainVerified sella la verificación del dominio. Acotado por (org,
// dominio): si el dominio cambió entre la consulta DNS y el sello, no marca nada.
// Idempotente: conserva el primer sello. Es el mismo sello que
// MarkOrgMailDomainVerified —el asistente y la pantalla de dominios no pueden
// tener dos maneras de dar por bueno un TXT—, y tolera que el dominio ya no esté
// porque quien lo reclamó pudo cambiarlo mientras se consultaba el DNS.
func (s *Store) MarkOrgDomainVerified(ctx context.Context, q pg.Querier, orgID, host string) error {
	err := s.MarkOrgMailDomainVerified(ctx, q, orgID, host)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	return err
}

// ClearOrgDomain desconecta el dominio propio: la organización vuelve a tener el
// correo en su subdominio de la suite.
//
// NO toca los buzones que ya existen. Quitarle la dirección a quien la está usando
// es perder su correo, no deshacer una configuración; y como la dirección del
// dominio base existe siempre por contrato (ver domain.OrgBaseMailDomain), nadie
// se queda ilocalizable. Lo que cambia es a partir de cuándo: los buzones NUEVOS
// vuelven a nacer en el subdominio y Sytue deja de afirmar que sirve ese dominio.
//
// El token TXT se borra con el dominio: si mañana se reconecta el mismo, se emite
// uno nuevo. Reutilizar el viejo dejaría un registro válido publicado en una zona
// que ya no controlamos.
// Con varios dominios conectados desconecta el PRINCIPAL, que es el que esa
// pantalla enseña, y el más antiguo de los que queden verificados hereda el
// puesto (ver DeleteOrgMailDomain).
func (s *Store) ClearOrgDomain(ctx context.Context, orgID string) error {
	var id string
	err := s.DB().QueryRow(ctx,
		`SELECT id::text FROM org_mail_domains WHERE org_id = $1 AND is_primary`, orgID).Scan(&id)
	if errors.Is(pgErr(err), domain.ErrNotFound) {
		// Ya no hay dominio propio principal: el estado que se pedía es el que hay.
		return nil
	}
	if err != nil {
		return errf("ClearOrgDomain", err)
	}
	return s.DeleteOrgMailDomain(ctx, orgID, id)
}

// --- Cloudflare del CLIENTE (mig 0028) --------------------------------------
//
// Credenciales de la cuenta de Cloudflare DEL CLIENTE, para el camino «mi dominio
// está en Cloudflare, configúralo por mí». Por defecto NO se guardan: el token se
// usa y se descarta. Solo llegan aquí si el cliente pide recordarlo, y entonces
// van SELLADAS con la KEK. A propósito NO forman parte de profileCols: el perfil
// se serializa en respuestas HTTP y este token no puede salir por ahí ni por
// descuido.

// SetOrgCloudflare guarda las credenciales de Cloudflare del cliente, con el
// token sellado con la KEK. Token vacío ⇒ las BORRA (es la vía por la que el
// cliente retira el permiso: desmarcar «recordar» olvida de verdad).
func (s *Store) SetOrgCloudflare(ctx context.Context, q pg.Querier, orgID, zoneID, token string) error {
	if q == nil {
		q = s.DB()
	}
	sealed := ""
	if token != "" {
		var err error
		if sealed, err = s.keyCipher.Encrypt(token); err != nil {
			return errf("SetOrgCloudflare.seal", err)
		}
	} else {
		zoneID = ""
	}
	const sql = `INSERT INTO org_profiles (org_id, cf_zone_id, cf_api_token)
		VALUES ($1, $2, $3)
		ON CONFLICT (org_id) DO UPDATE SET
			cf_zone_id = $2, cf_api_token = $3, updated_at = now()`
	_, err := q.Exec(ctx, sql, orgID, zoneID, sealed)
	return errf("SetOrgCloudflare", err)
}

// GetOrgCloudflare devuelve la zona y el token guardados (ya descifrados), o dos
// cadenas vacías si la organización no pidió recordarlos. Sin fila NO es error:
// no haber guardado nada es el estado normal.
func (s *Store) GetOrgCloudflare(ctx context.Context, orgID string) (zoneID, token string, err error) {
	var sealed string
	err = s.DB().QueryRow(ctx,
		`SELECT cf_zone_id, cf_api_token FROM org_profiles WHERE org_id = $1`, orgID).
		Scan(&zoneID, &sealed)
	switch e := errf("GetOrgCloudflare", err); {
	case e == nil:
	case errors.Is(e, domain.ErrNotFound):
		return "", "", nil
	default:
		return "", "", e
	}
	if sealed == "" {
		return zoneID, "", nil
	}
	token, err = s.keyCipher.Decrypt(sealed)
	if err != nil {
		return "", "", errf("GetOrgCloudflare.unseal", err)
	}
	return zoneID, token, nil
}

// PendingInvitation es una invitación VIGENTE de una organización (lo que el
// resumen del alta enseña: a quién se invitó de verdad y con qué rol).
type PendingInvitation struct {
	Email     string
	Role      string
	ExpiresAt time.Time
}

// ListPendingInvitations lista las invitaciones NO caducadas de la org. Las
// aceptadas ya no están (ConsumeInvitation las borra): lo que sale de aquí es
// exactamente «pendiente de aceptar».
func (s *Store) ListPendingInvitations(ctx context.Context, orgID string) ([]PendingInvitation, error) {
	const sql = `SELECT email, role, expires_at FROM org_invitations
		WHERE org_id = $1 AND expires_at > now() ORDER BY email`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("ListPendingInvitations", err)
	}
	defer rows.Close()
	out := make([]PendingInvitation, 0, 8)
	for rows.Next() {
		var p PendingInvitation
		if err := rows.Scan(&p.Email, &p.Role, &p.ExpiresAt); err != nil {
			return nil, errf("ListPendingInvitations.scan", err)
		}
		out = append(out, p)
	}
	return out, errf("ListPendingInvitations.iter", rows.Err())
}
