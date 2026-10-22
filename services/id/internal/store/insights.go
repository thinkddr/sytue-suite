// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"
)

// OrgInsights son los recuentos que responden «qué hay que mirar hoy» en una
// organización.
//
// Van juntos y en UNA consulta a propósito: cuatro viajes a la base para pintar
// una portada es lo que convierte un panel en algo que tarda, y lo que tarda se
// deja de mirar. Todos se calculan sobre `memberships`, así que la organización
// acota el conjunto sin depender de que cada subconsulta se acuerde.
type OrgInsights struct {
	// Miembros activos sin segundo factor. Es el número que decide si la
	// organización está a un phishing de distancia.
	SinMFA int
	// Cuentas suspendidas que siguen ahí: ni recuperadas ni dadas de baja.
	Suspendidos int
	// Direcciones sin verificar: no reciben avisos y no prueban ser de quien dicen.
	SinVerificar int
	// Invitaciones que caducaron sin que nadie las aceptara. Alguien se quedó
	// esperando, y nadie se entera hasta que lo dice.
	InvitacionesCaducadas int
	// Invitaciones vivas, para saber a cuánta gente se está esperando.
	InvitacionesVivas int
}

// OrgInsightsFor calcula los recuentos de una organización en una sola ida.
//
// FILTER en vez de cinco consultas: PostgreSQL recorre `memberships` una vez.
// Las invitaciones sí van aparte —otra tabla— pero en la misma llamada.
func (s *Store) OrgInsightsFor(ctx context.Context, orgID string, now time.Time) (OrgInsights, error) {
	var o OrgInsights
	const sql = `
		SELECT
			count(*) FILTER (WHERE u.status = 'active' AND NOT u.mfa_enabled),
			count(*) FILTER (WHERE u.status = 'suspended'),
			count(*) FILTER (WHERE u.status = 'active' AND NOT u.email_verified),
			(SELECT count(*) FROM org_invitations i WHERE i.org_id = $1 AND i.expires_at <= $2),
			(SELECT count(*) FROM org_invitations i WHERE i.org_id = $1 AND i.expires_at > $2)
		FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.org_id = $1`
	err := s.DB().QueryRow(ctx, sql, orgID, now).Scan(&o.SinMFA, &o.Suspendidos,
		&o.SinVerificar, &o.InvitacionesCaducadas, &o.InvitacionesVivas)
	return o, errf("OrgInsightsFor", err)
}
