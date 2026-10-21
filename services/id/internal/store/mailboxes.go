// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/mailaddr"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Buzones de organización: qué dirección de correo le corresponde a cada persona
// dentro de cada organización a la que pertenece, y sus alias. El porqué de que
// esto viva en Sytue ID y no solo en Sytue Mail está en la migración 0048.

// maxIntentosDireccion acota el desempate de direcciones. Con doce personas que se
// llaman exactamente igual en la misma organización, la decimotercera merece que un
// administrador le ponga la dirección a mano en vez de heredar un ana.perez13 que
// nadie va a acertar a la primera. Es un tope de cordura, no de rendimiento.
const maxIntentosDireccion = 12

// SetSuiteDomain fija el dominio de la suite (sytue.eu) que sirve de base a los
// subdominios de organización. No es un parámetro de cada llamada porque quien
// necesita el dato —EnsureOrgMailbox, colgando de UpsertMembership— está tres
// capas por debajo de donde vive la configuración, y hacerlo viajar habría obligado
// a tocar las ocho vías que crean membresías. Vacío ⇒ no se reparten buzones: es la
// situación de los subcomandos de línea de órdenes (seed, bootstrap), que no cargan
// la configuración del servidor.
func (s *Store) SetSuiteDomain(d string) { s.suiteDomain = d }

// MailDomainForOrg devuelve el dominio de correo de una organización: el suyo
// propio si está verificado y, si no, su subdominio de la suite. Devuelve cadena
// vacía si no hay dominio de suite configurado (ver SetSuiteDomain).
func (s *Store) MailDomainForOrg(ctx context.Context, q pg.Querier, orgID string) (string, error) {
	principal, _, err := s.MailDomainsForOrg(ctx, q, orgID)
	return principal, err
}

// MailDomainsForOrg devuelve los DOS dominios de una organización: el principal
// —el propio si está verificado, si no el de la suite— y el base, que es siempre
// el de la suite.
//
// Van juntos y no en dos consultas porque salen de la misma fila y porque quien
// asigna direcciones los necesita a la vez: la principal vive en el primero y la
// del segundo tiene que existir igualmente (ver domain.OrgBaseMailDomain). Cuando
// no hay dominio propio verificado los dos valen lo mismo, y entonces solo se
// asigna una dirección.
func (s *Store) MailDomainsForOrg(ctx context.Context, q pg.Querier, orgID string) (principal, base string, err error) {
	if s.suiteDomain == "" {
		return "", "", nil
	}
	if q == nil {
		q = s.DB()
	}
	var (
		slug     string
		propio   string
		verified bool
	)
	// LEFT JOIN: la mayoría de organizaciones no tiene ningún dominio propio. Sin
	// el LEFT JOIN, esas organizaciones se quedarían sin buzones y el fallo sería
	// mudo. De los suyos manda el PRINCIPAL: desde la mig 0052 puede tener varios,
	// y el principal es el que estrenan las direcciones nuevas.
	if err := q.QueryRow(ctx, `
		SELECT o.slug, coalesce(d.domain, ''), d.verified_at IS NOT NULL
		FROM organizations o
		LEFT JOIN org_mail_domains d ON d.org_id = o.id AND d.is_primary
		WHERE o.id = $1`, orgID).Scan(&slug, &propio, &verified); err != nil {
		return "", "", errf("MailDomainsForOrg", err)
	}
	return domain.OrgMailDomain(slug, propio, verified, s.suiteDomain),
		domain.OrgBaseMailDomain(slug, s.suiteDomain), nil
}

// EnsureOrgMailbox asigna —si aún no la tiene— la dirección principal de una
// persona en una organización, y la devuelve. Es IDEMPOTENTE: si ya tenía buzón se
// devuelve el que hay sin tocarlo, porque cambiar la dirección de alguien que ya
// recibe correo es perder correo, no corregir un dato.
//
// `deseada` es la dirección personalizada (la que eligió quien dio de alta); vacía
// ⇒ patrón nombre.apellidos. Si la deseada está cogida NO se falla el alta: se cae
// al patrón por defecto. Dejar a alguien sin cuenta porque su dirección preferida
// estaba pillada sería el peor de los dos males, y el administrador puede
// cambiarla después desde la consola.
//
// Devuelve cadena vacía sin error cuando no hay dominio de correo que repartir
// (sin dominio de suite configurado): el alta de la persona sigue siendo válida,
// simplemente todavía no hay buzón que darle.
func (s *Store) EnsureOrgMailbox(ctx context.Context, q pg.Querier, orgID, userID, deseada string) (string, error) {
	if q == nil {
		q = s.DB()
	}
	mailDomain, baseDomain, err := s.MailDomainsForOrg(ctx, q, orgID)
	if err != nil || mailDomain == "" {
		return "", err
	}
	if actual, err := s.primaryMailbox(ctx, q, orgID, userID); err != nil || actual != "" {
		// Ya tiene principal, pero puede faltarle la del dominio base: pasa con
		// quien entró ANTES de que la organización conectara su dominio propio, y
		// con quien entró después si el reparto se quedó a medias. Se comprueba
		// siempre —es una consulta por el índice— porque la garantía es que esa
		// dirección exista, no que existiera el día del alta.
		if err == nil {
			if berr := s.asegurarDireccionBase(ctx, q, orgID, userID, actual, baseDomain); berr != nil {
				return "", berr
			}
		}
		return actual, err
	}

	var given, family, name, email string
	if err := q.QueryRow(ctx,
		`SELECT given_name, family_name, name, email FROM users WHERE id = $1`,
		userID).Scan(&given, &family, &name, &email); err != nil {
		return "", errf("EnsureOrgMailbox.perfil", err)
	}

	// Orden de los intentos: primero la dirección personalizada (si la hay) y
	// después el patrón con sus desempates. Un solo bucle para las dos cosas, de
	// modo que la personalizada pase por la MISMA comprobación de unicidad.
	var intentos []string
	if deseada != "" {
		if addr, err := domain.NormalizeMailboxAddress(deseada, mailDomain); err == nil {
			intentos = append(intentos, addr)
		}
	}
	local := domain.MailboxLocalPart(given, family, name, email)
	if local == "" && len(intentos) == 0 {
		// Ni nombre utilizable ni dirección elegida: no hay nada que asignar. Antes
		// que inventar un identificador opaco que nadie sabría teclear, se deja sin
		// buzón para que la consola lo enseñe como pendiente y alguien lo decida.
		return "", nil
	}
	for n := 0; n < maxIntentosDireccion && local != ""; n++ {
		intentos = append(intentos, domain.MailboxCandidate(local, mailDomain, n))
	}

	for _, addr := range intentos {
		asignada, err := s.insertMailbox(ctx, q, orgID, userID, addr, true)
		if err != nil {
			return "", err
		}
		if asignada != "" {
			if berr := s.asegurarDireccionBase(ctx, q, orgID, userID, asignada, baseDomain); berr != nil {
				return "", berr
			}
			return asignada, nil
		}
	}
	// Todos los intentos chocaron. O bien hay doce homónimos, o bien otra
	// transacción asignó el buzón de esta misma persona mientras tanto (el índice
	// parcial de dirección principal rechaza el segundo). Releer distingue los dos
	// casos sin adivinar: si hay buzón, es el de la carrera y vale.
	return s.primaryMailbox(ctx, q, orgID, userID)
}

// asegurarDireccionBase garantiza que la persona tiene además una dirección en el
// subdominio de la suite, como alias, cuando su principal vive en el dominio propio
// de la organización.
//
// No es cortesía: es el cimiento del que cuelga todo lo que en la suite necesita
// una dirección que NOSOTROS servimos (ver domain.OrgBaseMailDomain). Sin ella,
// una organización con dominio propio deja a sus miembros sin ninguna dirección en
// un dominio nuestro, y el día que ese dominio caduca o cambia de MX el correo
// interno de la suite deja de entregar sin que nadie haya tocado nada.
//
// Reutiliza la parte local de la principal para que las dos direcciones de una
// persona se parezcan; si esa parte ya está cogida en el dominio base —dos
// organizaciones distintas pueden tener una ana.perez— desempata igual que el alta.
func (s *Store) asegurarDireccionBase(ctx context.Context, q pg.Querier, orgID, userID, principal, baseDomain string) error {
	if baseDomain == "" || principal == "" {
		return nil
	}
	local, dom, ok := strings.Cut(principal, "@")
	if !ok || local == "" || strings.EqualFold(dom, baseDomain) {
		// La principal YA vive en el dominio base: no hay nada que añadir.
		return nil
	}
	for n := 0; n < maxIntentosDireccion; n++ {
		addr := domain.MailboxCandidate(local, baseDomain, n)
		asignada, err := s.insertMailbox(ctx, q, orgID, userID, addr, false)
		if err != nil {
			return err
		}
		if asignada != "" {
			return nil
		}
		// Cadena vacía = la dirección ya existe. Si es de esta misma persona, la
		// garantía ya está cumplida y no hay que seguir probando desempates: sin
		// esta comprobación, cada llamada intentaría el n+1 y acabaría dándole
		// ana.perez2, ana.perez3… a alguien que ya tenía la suya.
		mia, err := s.tieneDireccion(ctx, q, orgID, userID, addr)
		if err != nil {
			return err
		}
		if mia {
			return nil
		}
	}
	return nil
}

// PromoteOrgMailDomain muda a TODA la organización al dominio principal recién
// verificado y devuelve a cuántas personas les cambió la dirección.
//
// Conectar un dominio y que solo lo estrenen los que entren a partir de mañana no
// es conectar un dominio: la plantilla que ya está dentro seguiría escribiendo
// desde el subdominio de la suite y el cliente vería que su compra no hizo nada.
//
// La dirección anterior NO se borra, se degrada a alias. Es la diferencia entre
// mudarse y perder el correo: quien escriba a la vieja sigue llegando, y todo lo
// que la suite tuviera apuntado ahí —una firma en un documento, un canal de
// Rooms— sigue resolviendo. Es además la dirección del dominio base, que por
// contrato existe siempre (ver domain.OrgBaseMailDomain).
//
// Es IDEMPOTENTE: quien ya esté en el dominio principal no se toca, así que
// repetirlo tras un fallo a mitad termina el trabajo en vez de duplicarlo.
func (s *Store) PromoteOrgMailDomain(ctx context.Context, orgID string) (int, error) {
	principal, base, err := s.MailDomainsForOrg(ctx, nil, orgID)
	if err != nil || principal == "" || strings.EqualFold(principal, base) {
		return 0, err
	}
	// Quiénes están fuera del dominio principal. Se lee ANTES y de una vez: hacerlo
	// dentro del bucle mientras se insertan filas es leer una lista que uno mismo
	// está cambiando.
	rows, err := s.DB().Query(ctx, `
		SELECT user_id::text, address FROM org_mailboxes
		WHERE org_id = $1 AND primary_address AND removed_at IS NULL
		  AND lower(split_part(address, '@', 2)) <> lower($2)`, orgID, principal)
	if err != nil {
		return 0, errf("PromoteOrgMailDomain.listar", err)
	}
	type pendiente struct{ userID, address string }
	var lista []pendiente
	for rows.Next() {
		var p pendiente
		if err := rows.Scan(&p.userID, &p.address); err != nil {
			rows.Close()
			return 0, errf("PromoteOrgMailDomain.scan", err)
		}
		lista = append(lista, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, errf("PromoteOrgMailDomain.rows", err)
	}

	movidos := 0
	for _, p := range lista {
		local, _, ok := strings.Cut(p.address, "@")
		if !ok || local == "" {
			continue
		}
		// Una transacción POR PERSONA y no una para todas: con una sola, un choque
		// de direcciones en la persona número doscientos abortaría también las
		// ciento noventa y nueve mudanzas que ya habían salido bien, y al reintentar
		// volvería a chocar en el mismo sitio. Así el fallo se queda en quien lo
		// causa y el resto se muda.
		err := s.InTx(ctx, func(q pg.Querier) error {
			// Degradar ANTES de insertar: el índice único parcial solo admite una
			// principal por persona y organización, así que insertar primero
			// chocaría siempre consigo mismo.
			if _, err := q.Exec(ctx, `
				UPDATE org_mailboxes SET primary_address = false
				WHERE org_id = $1 AND user_id = $2 AND primary_address AND removed_at IS NULL`,
				orgID, p.userID); err != nil {
				return errf("PromoteOrgMailDomain.degradar", err)
			}
			for n := 0; n < maxIntentosDireccion; n++ {
				addr := domain.MailboxCandidate(local, principal, n)
				asignada, err := s.insertMailbox(ctx, q, orgID, p.userID, addr, true)
				if err != nil {
					return err
				}
				if asignada != "" {
					return nil
				}
			}
			// Doce homónimos en el dominio nuevo: se deshace la degradación
			// devolviendo el error, y esta persona se queda como estaba en vez de
			// quedarse sin ninguna dirección principal —que es lo que la dejaría
			// sin poder enviar—.
			return domain.ErrConflict
		})
		switch {
		case err == nil:
			movidos++
		case errors.Is(err, domain.ErrConflict):
			// Se salta y se sigue: el resto de la organización sí se muda.
			continue
		default:
			return movidos, err
		}
	}
	return movidos, nil
}

// tieneDireccion dice si esa dirección concreta ya es de esa persona.
func (s *Store) tieneDireccion(ctx context.Context, q pg.Querier, orgID, userID, address string) (bool, error) {
	var existe bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM org_mailboxes
			WHERE org_id = $1 AND user_id = $2 AND lower(address) = lower($3)
			  AND removed_at IS NULL
		)`, orgID, userID, address).Scan(&existe)
	return existe, errf("tieneDireccion", err)
}

// primaryMailbox devuelve la dirección principal ya asignada, o vacío si no hay.
func (s *Store) primaryMailbox(ctx context.Context, q pg.Querier, orgID, userID string) (string, error) {
	var addr string
	err := q.QueryRow(ctx,
		`SELECT address FROM org_mailboxes
		 WHERE org_id = $1 AND user_id = $2 AND primary_address AND removed_at IS NULL`,
		orgID, userID).Scan(&addr)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return addr, errf("primaryMailbox", err)
}

// ReserveMailbox reserva EXACTAMENTE esa dirección como principal de la persona,
// o ErrConflict si ya es de alguien. Es lo contrario de EnsureOrgMailbox, que ante
// una dirección cogida cae al patrón: el alta por @usuario no puede darle a nadie
// una dirección distinta de la que eligió (ADR 0040).
func (s *Store) ReserveMailbox(ctx context.Context, q pg.Querier, orgID, userID, address string) error {
	if q == nil {
		q = s.DB()
	}
	got, err := s.insertMailbox(ctx, q, orgID, userID, strings.ToLower(address), true)
	if err != nil {
		return err
	}
	if got == "" {
		return domain.ErrConflict
	}
	return nil
}

// insertMailbox intenta reservar una dirección. Devuelve la dirección si la
// reserva prosperó y cadena vacía si ya estaba cogida (por esta persona o por
// cualquier otra de cualquier organización: el espacio de direcciones es único).
//
// ON CONFLICT DO NOTHING sin especificar el índice, a propósito: los dos índices
// únicos de la tabla (dirección global y una sola principal por persona) son
// motivos legítimos para no insertar, y nombrar uno solo dejaría que el otro
// abortara la transacción entera del alta.
func (s *Store) insertMailbox(ctx context.Context, q pg.Querier, orgID, userID, address string, primary bool) (string, error) {
	var out string
	err := q.QueryRow(ctx, `
		INSERT INTO org_mailboxes (org_id, user_id, address, primary_address)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING
		RETURNING address`, orgID, userID, address, primary).Scan(&out)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return out, errf("insertMailbox", err)
}

// ListOrgMailboxes devuelve todas las direcciones de una organización, o las de una
// sola persona si userID no viene vacío. La principal primero y los alias después,
// que es como los enseña la consola.
func (s *Store) ListOrgMailboxes(ctx context.Context, orgID, userID string) ([]domain.OrgMailbox, error) {
	rows, err := s.DB().Query(ctx, `
		SELECT id::text, user_id::text, address, primary_address
		FROM org_mailboxes
		WHERE org_id = $1 AND ($2 = '' OR user_id = $2::uuid) AND removed_at IS NULL
		ORDER BY user_id, primary_address DESC, address`, orgID, userID)
	if err != nil {
		return nil, errf("ListOrgMailboxes", err)
	}
	defer rows.Close()
	var out []domain.OrgMailbox
	for rows.Next() {
		var m domain.OrgMailbox
		if err := rows.Scan(&m.ID, &m.UserID, &m.Address, &m.Primary); err != nil {
			return nil, errf("ListOrgMailboxes.scan", err)
		}
		out = append(out, m)
	}
	return out, errf("ListOrgMailboxes.rows", rows.Err())
}

// ListOrgMailboxesDe devuelve las direcciones de UN CONJUNTO de personas, en una
// sola consulta y agrupadas por quién.
//
// Existe por la agenda: la página del directorio trae hasta 500 personas, y
// resolver sus direcciones una a una serían 500 viajes a la base por cada carga.
// `ListOrgMailboxes` con el usuario vacío tampoco vale: devuelve las de TODA la
// OrgUserIDForKey resuelve la «userKey» de las APIs de Google dentro de una
// organización: el id del usuario, una de sus direcciones de la organización o su
// correo de registro. Solo devuelve miembros de ESA organización; ErrNotFound si no.
func (s *Store) OrgUserIDForKey(ctx context.Context, orgID, key string) (string, error) {
	var id string
	err := s.DB().QueryRow(ctx, `
		SELECT m.user_id::text
		  FROM memberships m JOIN users u ON u.id = m.user_id
		 WHERE m.org_id = $1
		   AND (m.user_id::text = $2
		        OR lower(u.email) = lower($2)
		        OR EXISTS (SELECT 1 FROM org_mailboxes b
		                    WHERE b.org_id = m.org_id AND b.user_id = m.user_id
		                      AND b.removed_at IS NULL AND lower(b.address) = lower($2)))
		 LIMIT 1`, orgID, key).Scan(&id)
	return id, errf("OrgUserIDForKey", err)
}

// organización, que en una grande son miles de filas de las que se usan 500.
func (s *Store) ListOrgMailboxesDe(ctx context.Context, orgID string, userIDs []string) (map[string][]domain.OrgMailbox, error) {
	out := make(map[string][]domain.OrgMailbox, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := s.DB().Query(ctx, `
		SELECT id::text, user_id::text, address, primary_address
		FROM org_mailboxes
		WHERE org_id = $1 AND user_id = ANY($2::uuid[]) AND removed_at IS NULL
		ORDER BY user_id, primary_address DESC, address`, orgID, userIDs)
	if err != nil {
		return nil, errf("ListOrgMailboxesDe", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m domain.OrgMailbox
		if err := rows.Scan(&m.ID, &m.UserID, &m.Address, &m.Primary); err != nil {
			return nil, errf("ListOrgMailboxesDe.scan", err)
		}
		out[m.UserID] = append(out[m.UserID], m)
	}
	return out, errf("ListOrgMailboxesDe.rows", rows.Err())
}

// dominioAsignable dice si una dirección puede repartirse en esta organización:
// tiene que estar en su subdominio de la suite o en uno de sus dominios propios
// VERIFICADOS.
//
// Hasta la mig 0052 no hacía falta comprobarlo —solo había un dominio y la
// dirección se componía con él—, pero NormalizeMailboxAddress acepta la dirección
// entera si trae arroba, así que una escrita a mano en la consola entraba en
// cualquier dominio: se reservaba en nuestro espacio de direcciones, Sytue Mail
// la rechazaba por ajena y el reconciliador se quedaba reintentándola. Con varios
// dominios propios la comprobación ya no es opcional: hay que saber cuáles son.
func (s *Store) dominioAsignable(ctx context.Context, q pg.Querier, orgID, address string) (bool, error) {
	_, dom, ok := strings.Cut(address, "@")
	if !ok || dom == "" {
		return false, nil
	}
	principal, base, err := s.MailDomainsForOrg(ctx, q, orgID)
	if err != nil {
		return false, err
	}
	if strings.EqualFold(dom, principal) || strings.EqualFold(dom, base) {
		return true, nil
	}
	return s.OrgOwnsMailDomain(ctx, orgID, dom)
}

// AddMailboxAlias da de alta una dirección adicional que entrega en el buzón de esa
// persona. La unicidad la resuelve el índice: si la dirección ya es de alguien
// —buzón principal o alias, de esta organización o de otra— devuelve ErrConflict en
// vez de secuestrarla.
//
// Solo se permite alias de quien YA tiene buzón principal: un alias es otra puerta
// a un buzón, y sin buzón no hay puerta que abrir. Sin esa comprobación se podrían
// crear direcciones que resuelven a una persona sin cuenta en Sytue Mail, que es
// exactamente el rebote silencioso que la migración 0048 viene a quitar.
func (s *Store) AddMailboxAlias(ctx context.Context, orgID, userID, entrada string) (domain.OrgMailbox, error) {
	mailDomain, err := s.MailDomainForOrg(ctx, nil, orgID)
	if err != nil {
		return domain.OrgMailbox{}, err
	}
	if mailDomain == "" {
		return domain.OrgMailbox{}, domain.ErrInvalidInput
	}
	address, err := domain.NormalizeMailboxAddress(entrada, mailDomain)
	if err != nil {
		return domain.OrgMailbox{}, err
	}
	// En el dominio raíz (cuentas personales y la propia Sytue) nadie se queda con
	// admin@ ni soporte@: sería phishing con nuestra firma. Misma lista que el
	// autoservicio de Sytue Mail (platform/pkg/mailaddr).
	if local, dom, ok := strings.Cut(address, "@"); ok && mailaddr.Suplanta(local, dom, s.suiteDomain) {
		return domain.OrgMailbox{}, domain.ErrAliasReservado
	}
	// El alias puede ir en CUALQUIER dominio de la organización (una empresa con
	// dos marcas quiere ventas@ en las dos), pero solo en los suyos.
	if ok, err := s.dominioAsignable(ctx, nil, orgID, address); err != nil || !ok {
		if err != nil {
			return domain.OrgMailbox{}, err
		}
		return domain.OrgMailbox{}, domain.ErrInvalidInput
	}
	principal, err := s.primaryMailbox(ctx, s.DB(), orgID, userID)
	if err != nil {
		return domain.OrgMailbox{}, err
	}
	if principal == "" {
		return domain.OrgMailbox{}, domain.ErrNotFound
	}
	var m domain.OrgMailbox
	err = s.DB().QueryRow(ctx, `
		INSERT INTO org_mailboxes (org_id, user_id, address, primary_address)
		VALUES ($1, $2, $3, false)
		ON CONFLICT DO NOTHING
		RETURNING id::text, user_id::text, address, primary_address`,
		orgID, userID, address).Scan(&m.ID, &m.UserID, &m.Address, &m.Primary)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OrgMailbox{}, domain.ErrConflict
	}
	return m, errf("AddMailboxAlias", err)
}

// DeleteMailboxAlias borra un alias de la organización. NUNCA borra una dirección
// principal: quitarle a alguien su dirección lo deja sin poder recibir, y eso no
// puede ser el efecto de pulsar «eliminar» en una lista de alias. Acotado por
// org_id para que un administrador no alcance el alias de otra organización.
// No es un DELETE sino una MARCA: la dirección también existe en Sytue Mail y allí
// seguiría entregando. Quien la retira creería haberla quitado y el correo seguiría
// llegando. El reconciliador la retira también allí y entonces desaparece la fila.
func (s *Store) DeleteMailboxAlias(ctx context.Context, orgID, id string) error {
	tag, err := s.DB().Exec(ctx, `
		UPDATE org_mailboxes SET removed_at = now()
		WHERE id = $1 AND org_id = $2 AND NOT primary_address AND removed_at IS NULL`, id, orgID)
	if err != nil {
		return errf("DeleteMailboxAlias", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// PendingMailboxRemovals devuelve las direcciones marcadas para retirar que aún
// existen en Sytue Mail. Las que nunca llegaron a crearse allí (provisioned_at
// NULL) no hacen falta aquí: no hay nada que retirar y las purga PurgeMailbox.
func (s *Store) PendingMailboxRemovals(ctx context.Context, limit int) ([]MailboxPendiente, error) {
	rows, err := s.DB().Query(ctx, `
		SELECT m.id::text, m.user_id::text, o.slug, m.address, u.name, m.primary_address,
		       coalesce(p.address, '')
		FROM org_mailboxes m
		JOIN organizations o ON o.id = m.org_id
		JOIN users u ON u.id = m.user_id
		LEFT JOIN org_mailboxes p
		       ON p.org_id = m.org_id AND p.user_id = m.user_id AND p.primary_address
		      AND p.removed_at IS NULL
		WHERE m.removed_at IS NOT NULL
		ORDER BY m.removed_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, errf("PendingMailboxRemovals", err)
	}
	defer rows.Close()
	var out []MailboxPendiente
	for rows.Next() {
		var p MailboxPendiente
		if err := rows.Scan(&p.ID, &p.UserID, &p.OrgSlug, &p.Address, &p.FullName,
			&p.Primary, &p.PrimaryAddress); err != nil {
			return nil, errf("PendingMailboxRemovals.scan", err)
		}
		out = append(out, p)
	}
	return out, errf("PendingMailboxRemovals.rows", rows.Err())
}

// PurgeMailbox borra la fila de una dirección ya retirada de Sytue Mail. Hasta este
// momento seguía ocupando su sitio en el índice único, que es lo que impide
// reasignarla a otra persona antes de que deje de entregar en el buzón anterior.
func (s *Store) PurgeMailbox(ctx context.Context, id string) error {
	_, err := s.DB().Exec(ctx,
		`DELETE FROM org_mailboxes WHERE id = $1 AND removed_at IS NOT NULL`, id)
	return errf("PurgeMailbox", err)
}

// SetPrimaryMailbox cambia la dirección principal de una persona. La anterior NO se
// borra: se degrada a alias, para que el correo que ya iba a la dirección vieja
// —remitentes que la tienen guardada, listas, formularios— siga entregándose. Un
// cambio de dirección que hace desaparecer la anterior es una pérdida de correo
// disfrazada de renombrado.
func (s *Store) SetPrimaryMailbox(ctx context.Context, orgID, userID, entrada string) (string, error) {
	mailDomain, err := s.MailDomainForOrg(ctx, nil, orgID)
	if err != nil {
		return "", err
	}
	if mailDomain == "" {
		return "", domain.ErrInvalidInput
	}
	address, err := domain.NormalizeMailboxAddress(entrada, mailDomain)
	if err != nil {
		return "", err
	}
	// Igual que un alias: la dirección principal solo puede vivir en un dominio de
	// la organización, y con varios ya no basta con haberla compuesto con el suyo.
	if ok, err := s.dominioAsignable(ctx, nil, orgID, address); err != nil || !ok {
		if err != nil {
			return "", err
		}
		return "", domain.ErrInvalidInput
	}
	err = s.InTx(ctx, func(q pg.Querier) error {
		// Degradar la principal actual ANTES de promover la nueva: el índice parcial
		// solo tolera una principal por persona, así que el orden inverso chocaría
		// consigo mismo.
		if _, e := q.Exec(ctx, `
			UPDATE org_mailboxes SET primary_address = false
			WHERE org_id = $1 AND user_id = $2 AND primary_address AND lower(address) <> lower($3)`,
			orgID, userID, address); e != nil {
			return errf("SetPrimaryMailbox.degradar", e)
		}
		// La dirección puede ser ya un alias de esta persona (se promociona) o nueva
		// (se inserta). Un UPSERT sobre el índice de dirección resuelve los dos casos,
		// pero solo si la dirección es SUYA: el WHERE impide promover a principal la
		// dirección de otra persona, que sería robársela.
		tag, e := q.Exec(ctx, `
			INSERT INTO org_mailboxes (org_id, user_id, address, primary_address)
			VALUES ($1, $2, $3, true)
			ON CONFLICT (lower(address)) DO UPDATE SET primary_address = true, updated_at = now()
			WHERE org_mailboxes.org_id = $1 AND org_mailboxes.user_id = $2`,
			orgID, userID, address)
		if e != nil {
			return errf("SetPrimaryMailbox.promover", e)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrConflict
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return address, nil
}

// MailboxPendiente es una dirección ya asignada que todavía no tiene cuenta en
// Sytue Mail. Lleva lo justo para poder crearla allí: a quién pertenece, en qué
// organización (el tenant del token) y con qué nombre mostrarla.
type MailboxPendiente struct {
	ID       string
	UserID   string
	OrgSlug  string
	Address  string
	FullName string
	// Primary distingue la dirección que CREA la cuenta de los alias, que solo
	// añaden una puerta más a una cuenta que ya existe. El reconciliador los empuja
	// por caminos distintos y necesita saber cuál es cuál.
	Primary bool
	// PrimaryAddress es la dirección principal del dueño del alias. La necesita el
	// reconciliador para autenticarse como él al registrar el alias (los alias son
	// del dueño de la cuenta en Sytue Mail); vacía en las filas principales.
	PrimaryAddress string
}

// PendingMailboxes devuelve las direcciones que aún no existen en Sytue Mail:
// principales (que crean la cuenta) y alias. Excluye a quien esté dado de baja,
// para no crear buzones a suspendidos.
//
// El orden por created_at importa: la dirección principal de alguien se asigna
// antes que sus alias, así que se empuja antes y el alias encuentra la cuenta ya
// creada. Si aun así llegara a destiempo, su fila se queda sin sello y el siguiente
// barrido lo reintenta.
func (s *Store) PendingMailboxes(ctx context.Context, limit int) ([]MailboxPendiente, error) {
	rows, err := s.DB().Query(ctx, `
		SELECT m.id::text, m.user_id::text, o.slug, m.address, u.name, m.primary_address,
		       coalesce(p.address, '')
		FROM org_mailboxes m
		JOIN organizations o ON o.id = m.org_id
		JOIN users u ON u.id = m.user_id
		LEFT JOIN org_mailboxes p
		       ON p.org_id = m.org_id AND p.user_id = m.user_id AND p.primary_address
		      AND p.removed_at IS NULL
		WHERE m.provisioned_at IS NULL AND m.removed_at IS NULL
		  AND u.status = 'active' AND o.status = 'active'
		ORDER BY m.created_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, errf("PendingMailboxes", err)
	}
	defer rows.Close()
	var out []MailboxPendiente
	for rows.Next() {
		var p MailboxPendiente
		if err := rows.Scan(&p.ID, &p.UserID, &p.OrgSlug, &p.Address, &p.FullName,
			&p.Primary, &p.PrimaryAddress); err != nil {
			return nil, errf("PendingMailboxes.scan", err)
		}
		out = append(out, p)
	}
	return out, errf("PendingMailboxes.rows", rows.Err())
}

// MarkMailboxProvisioned anota que la cuenta ya existe en Sytue Mail y saca la fila
// de la cola del reconciliador.
func (s *Store) MarkMailboxProvisioned(ctx context.Context, id string) error {
	_, err := s.DB().Exec(ctx,
		`UPDATE org_mailboxes SET provisioned_at = now() WHERE id = $1`, id)
	return errf("MarkMailboxProvisioned", err)
}

// MailboxesByUser devuelve, para una lista de personas de una organización, su
// dirección principal. Lo usa el listado de usuarios de la consola para no hacer
// una consulta por fila.
func (s *Store) MailboxesByUser(ctx context.Context, orgID string, userIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := s.DB().Query(ctx, `
		SELECT user_id::text, address FROM org_mailboxes
		WHERE org_id = $1 AND primary_address AND removed_at IS NULL
		  AND user_id = ANY($2::uuid[])`,
		orgID, userIDs)
	if err != nil {
		return nil, errf("MailboxesByUser", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, addr string
		if err := rows.Scan(&id, &addr); err != nil {
			return nil, errf("MailboxesByUser.scan", err)
		}
		out[id] = addr
	}
	return out, errf("MailboxesByUser.rows", rows.Err())
}

// BuzonesDeLaPersona son las direcciones principales de una persona en TODAS las
// organizaciones de las que es miembro y siguen activas: el claim `mailboxes` con el
// que el Hub le enseña sus avisos de cualquiera de ellas (ADR 0053). Se deriva en cada
// emisión de token, así que quien deja de ser miembro deja de tenerlo en el siguiente.
//
// ponytail: hasta diez, porque el token viaja dentro de la cookie de sesión de cada
// superficie; con más organizaciones, las demás se ven desde la propia.
func (s *Store) BuzonesDeLaPersona(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.DB().Query(ctx, `
		SELECT mb.address
		FROM org_mailboxes mb
		JOIN memberships m ON m.org_id = mb.org_id AND m.user_id = mb.user_id
		JOIN organizations o ON o.id = mb.org_id AND o.status = 'active'
		WHERE mb.user_id = $1 AND mb.primary_address AND mb.removed_at IS NULL
		ORDER BY mb.address
		LIMIT 10`, userID)
	if err != nil {
		return nil, errf("BuzonesDeLaPersona", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, errf("BuzonesDeLaPersona", err)
		}
		out = append(out, a)
	}
	return out, errf("BuzonesDeLaPersona", rows.Err())
}
