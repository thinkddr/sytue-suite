// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Dominios de correo propios de una organización (mig 0052). Antes cabía uno solo
// —tres columnas en org_profiles—, y conectar el segundo significaba desconectar
// el primero y con él las direcciones que ya usaba la plantilla.
//
// EL PRINCIPAL es el que estrenan las altas nuevas; los demás reciben y sirven
// para enviar-como. El subdominio de la suite NO está aquí: existe siempre y no
// se verifica ni se quita (ver domain.OrgBaseMailDomain).

// OrgMailDomain es un dominio propio de una organización.
type OrgMailDomain struct {
	ID         string
	Domain     string
	Token      string
	VerifiedAt *time.Time
	Primary    bool
	CreatedAt  time.Time
}

// Verified indica si consta la titularidad del dominio.
func (d OrgMailDomain) Verified() bool { return d.VerifiedAt != nil }

const mailDomainCols = `id::text, domain, token, verified_at, is_primary, created_at`

func scanMailDomain(row pgx.Row) (OrgMailDomain, error) {
	var d OrgMailDomain
	err := row.Scan(&d.ID, &d.Domain, &d.Token, &d.VerifiedAt, &d.Primary, &d.CreatedAt)
	return d, err
}

// ListOrgMailDomains devuelve los dominios de la organización, el principal
// primero y el resto por antigüedad: es el orden en que se leen en pantalla, y
// el principal es la respuesta a la pregunta que se hace quien entra.
func (s *Store) ListOrgMailDomains(ctx context.Context, orgID string) ([]OrgMailDomain, error) {
	rows, err := s.DB().Query(ctx, `SELECT `+mailDomainCols+`
		FROM org_mail_domains WHERE org_id = $1
		ORDER BY is_primary DESC, created_at, domain`, orgID)
	if err != nil {
		return nil, errf("ListOrgMailDomains", err)
	}
	defer rows.Close()
	var out []OrgMailDomain
	for rows.Next() {
		d, err := scanMailDomain(rows)
		if err != nil {
			return nil, errf("ListOrgMailDomains.scan", err)
		}
		out = append(out, d)
	}
	return out, errf("ListOrgMailDomains.rows", rows.Err())
}

// VerifiedOrgMailDomains devuelve los dominios cuya titularidad consta. Es lo que
// viaja en el token hacia el resto de la suite: Sytue Mail no puede deducir de
// una dirección qué otros dominios sirve esa organización, y sin la lista
// rechazaba como ajena una dirección que la propia organización acababa de
// conectar (ver domain.AliasDomainAllowed en services/mail).
func (s *Store) VerifiedOrgMailDomains(ctx context.Context, q pg.Querier, orgID string) ([]string, error) {
	if q == nil {
		q = s.DB()
	}
	rows, err := q.Query(ctx, `SELECT domain FROM org_mail_domains
		WHERE org_id = $1 AND verified_at IS NOT NULL
		ORDER BY is_primary DESC, created_at, domain`, orgID)
	if err != nil {
		return nil, errf("VerifiedOrgMailDomains", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, errf("VerifiedOrgMailDomains.scan", err)
		}
		out = append(out, d)
	}
	return out, errf("VerifiedOrgMailDomains.rows", rows.Err())
}

// OrgOwnsMailDomain indica si el dominio es de la organización y está verificado.
// Es el permiso que se pide antes de asignar una dirección en él: sin esta
// comprobación, un administrador podría repartir buzones en un dominio que su
// organización no ha demostrado tener.
func (s *Store) OrgOwnsMailDomain(ctx context.Context, orgID, host string) (bool, error) {
	var ok bool
	err := s.DB().QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM org_mail_domains
		WHERE org_id = $1 AND domain = lower($2) AND verified_at IS NOT NULL)`,
		orgID, host).Scan(&ok)
	return ok, errf("OrgOwnsMailDomain", err)
}

// AddOrgMailDomain reclama un dominio para la organización con su token de
// verificación. IDEMPOTENTE: si ya estaba reclamado conserva su token y su sello
// —reintentar la comprobación no puede invalidar el registro TXT que el cliente
// ya publicó, ni reiniciar una verificación que ya salió bien—.
//
// Dominio ya verificado por OTRA organización ⇒ ErrConflict: quien publicó el TXT
// primero se lo queda. Reclamarlo sin verificar no bloquea a nadie, y por eso el
// choque solo salta contra los verificados (índice parcial de la mig 0052).
func (s *Store) AddOrgMailDomain(ctx context.Context, q pg.Querier, orgID, host, token string) (OrgMailDomain, error) {
	if q == nil {
		q = s.DB()
	}
	// El índice único de verificados NO basta para rechazar esto, y ahí estaba el
	// agujero: es PARCIAL (`WHERE verified_at IS NOT NULL`), o sea que solo impide
	// que DOS filas verificadas digan el mismo dominio. Una reclamación nueva nace
	// sin verificar, no entra en ese índice y no choca con nada: cualquier
	// organización podía reclamar el dominio que otra ya había demostrado suyo.
	//
	// Por eso la condición va en la propia sentencia y no en un SELECT previo: leer
	// y luego escribir deja la ventana abierta entre las dos. Excluye `org_id <> $1`
	// para que la organización dueña pueda seguir re-reclamando el suyo, que es el
	// camino idempotente de «comprobar ahora».
	d, err := scanMailDomain(q.QueryRow(ctx, `
		INSERT INTO org_mail_domains (org_id, domain, token)
		SELECT $1, lower($2), $3
		 WHERE NOT EXISTS (
		     SELECT 1 FROM org_mail_domains
		      WHERE domain = lower($2) AND verified_at IS NOT NULL AND org_id <> $1)
		ON CONFLICT (org_id, domain) DO UPDATE SET
			token = CASE WHEN org_mail_domains.token <> '' THEN org_mail_domains.token ELSE $3 END,
			updated_at = now()
		RETURNING `+mailDomainCols, orgID, host, token))
	// Sin filas significa que el NOT EXISTS lo paró: el dominio es de otra. No es
	// «no encontrado» —lo que falta no es la fila que se pide, es el permiso para
	// crearla— y quien llama distingue los dos para decir una cosa u otra.
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgMailDomain{}, fmt.Errorf("store: AddOrgMailDomain: %w", domain.ErrConflict)
	}
	return d, errf("AddOrgMailDomain", err)
}

// MarkOrgMailDomainVerified sella la titularidad de un dominio de la
// organización. Idempotente: conserva el primer sello.
//
// Y si la organización no tenía ninguno principal, este pasa a serlo: verificar
// el primer dominio propio es, en la práctica, estrenarlo. Sin esto, conectar un
// dominio dejaba las altas nuevas naciendo en el subdominio de la suite y nada en
// la pantalla lo explicaba.
func (s *Store) MarkOrgMailDomainVerified(ctx context.Context, q pg.Querier, orgID, host string) error {
	exec := func(q pg.Querier) error {
		tag, err := q.Exec(ctx, `
			UPDATE org_mail_domains
			SET verified_at = coalesce(verified_at, now()),
			    is_primary = is_primary OR NOT EXISTS (
			        SELECT 1 FROM org_mail_domains o WHERE o.org_id = $1 AND o.is_primary),
			    updated_at = now()
			WHERE org_id = $1 AND domain = lower($2)`, orgID, host)
		if err != nil {
			return errf("MarkOrgMailDomainVerified", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	}
	if q != nil {
		return exec(q)
	}
	return s.InTx(ctx, exec)
}

// SetPrimaryOrgMailDomain elige en qué dominio nacen las direcciones nuevas.
//
// Solo un dominio VERIFICADO puede ser principal: hacerlo principal es empezar a
// repartir direcciones en él, y repartirlas en un dominio del que no consta la
// titularidad crea direcciones que nuestro MX no serviría y que además pisarían
// las de su dueño real.
//
// Devuelve el dominio ya principal. Quien llama se encarga de mudar a la
// plantilla (PromoteOrgMailDomain): aquí solo se decide, porque mudar es una
// operación larga que no cabe en la transacción que sostiene este índice.
func (s *Store) SetPrimaryOrgMailDomain(ctx context.Context, orgID, id string) (OrgMailDomain, error) {
	var out OrgMailDomain
	err := s.InTx(ctx, func(q pg.Querier) error {
		// Degradar ANTES de promover: el índice parcial solo tolera un principal
		// por organización, así que el orden inverso chocaría consigo mismo.
		if _, err := q.Exec(ctx, `
			UPDATE org_mail_domains SET is_primary = false, updated_at = now()
			WHERE org_id = $1 AND is_primary AND id <> $2`, orgID, id); err != nil {
			return errf("SetPrimaryOrgMailDomain.degradar", err)
		}
		// Sin fila: o no es suyo, o no está verificado (errf lo traduce a
		// ErrNotFound). Los dos son «aquí no hay un dominio que puedas estrenar», y
		// distinguirlos le diría a quien pregunta si existe un dominio de otra
		// organización.
		d, err := scanMailDomain(q.QueryRow(ctx, `
			UPDATE org_mail_domains SET is_primary = true, updated_at = now()
			WHERE id = $1 AND org_id = $2 AND verified_at IS NOT NULL
			RETURNING `+mailDomainCols, id, orgID))
		out = d
		return errf("SetPrimaryOrgMailDomain", err)
	})
	return out, err
}

// DeleteOrgMailDomain desconecta un dominio de la organización.
//
// NO toca las direcciones ya asignadas en él. Quitarle la dirección a quien la
// está usando es perder su correo, no deshacer una configuración; y como la
// dirección del subdominio de la suite existe siempre por contrato (ver
// domain.OrgBaseMailDomain), nadie se queda ilocalizable. Lo que cambia es a
// partir de cuándo: las direcciones NUEVAS dejan de nacer ahí y Sytue deja de
// afirmar que sirve ese dominio.
//
// Si el que se va era el principal, hereda el verificado más antiguo que quede.
// Dejar a una organización con dominios verificados y ninguno principal la
// devolvería en silencio al subdominio de la suite para sus altas nuevas, que es
// justo lo que nadie espera después de quitar UNO de sus dominios.
func (s *Store) DeleteOrgMailDomain(ctx context.Context, orgID, id string) error {
	return s.InTx(ctx, func(q pg.Querier) error {
		var eraPrincipal bool
		// Sin fila ⇒ ErrNotFound por errf: no existe o no es de esta organización.
		if err := errf("DeleteOrgMailDomain", q.QueryRow(ctx, `
			DELETE FROM org_mail_domains WHERE id = $1 AND org_id = $2
			RETURNING is_primary`, id, orgID).Scan(&eraPrincipal)); err != nil {
			return err
		}
		if !eraPrincipal {
			return nil
		}
		_, err := q.Exec(ctx, `
			UPDATE org_mail_domains SET is_primary = true, updated_at = now()
			WHERE id = (
				SELECT id FROM org_mail_domains
				WHERE org_id = $1 AND verified_at IS NOT NULL
				ORDER BY created_at, domain LIMIT 1)`, orgID)
		return errf("DeleteOrgMailDomain.heredar", err)
	})
}
