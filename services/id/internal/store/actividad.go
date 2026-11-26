// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"
)

// SucesoDeCuenta es una fila del log de auditoría que la PROPIA persona ve en «Mi
// cuenta → Actividad reciente»: qué pasó, cuándo y desde qué dirección. No lleva el
// `detail` ni el `target`: son identificadores internos que no le dicen nada y que
// no hace falta sacar del servicio.
type SucesoDeCuenta struct {
	ID         int64
	OccurredAt time.Time
	Action     string
	IP         string
}

// ActividadDePersona devuelve los sucesos de `acciones` cuyo actor es `userID`, de
// los últimos `dias`, del más reciente al más antiguo y como mucho `limite`.
//
// Por actor y no por organización: la cuenta es de la persona y cruza todas sus
// organizaciones (un inicio de sesión en una, un cambio de contraseña desde otra).
// La lista de acciones la fija el llamante, y no se le deja a quien pregunta: el log
// guarda también lo que una persona hace como administradora, y eso no es actividad
// de SU cuenta.
//
// El índice `audit_log_actor_idx` (0001) acota por actor; el orden se resuelve sobre
// las filas de una sola persona, que son pocas, así que no pide un índice nuevo.
func (s *Store) ActividadDePersona(ctx context.Context, userID string, acciones []string, dias, limite int) ([]SucesoDeCuenta, error) {
	const sql = `SELECT id, occurred_at, action, coalesce(host(ip), '')
		FROM audit_log
		WHERE actor_id = $1::uuid AND action = ANY($2::text[])
		  AND occurred_at >= now() - make_interval(days => $3)
		ORDER BY occurred_at DESC, id DESC
		LIMIT $4`
	rows, err := s.DB().Query(ctx, sql, userID, acciones, dias, limite)
	if err != nil {
		return nil, errf("ActividadDePersona", err)
	}
	defer rows.Close()
	out := []SucesoDeCuenta{}
	for rows.Next() {
		var e SucesoDeCuenta
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.Action, &e.IP); err != nil {
			return nil, errf("ActividadDePersona.scan", err)
		}
		out = append(out, e)
	}
	return out, errf("ActividadDePersona.iter", rows.Err())
}
