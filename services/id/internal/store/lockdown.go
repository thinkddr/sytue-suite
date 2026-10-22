// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Bloqueo de emergencia (mig 0035). Fila única: el corte es del sistema entero.
//
// Se consulta en el camino de EMISIÓN —abrir sesión y entregar tokens—, no en
// la validación de cada petición: un token ya emitido lo invalida `revoke-all`
// rotando la clave de firma, y una consulta por petición convertiría al IdP en
// dependencia dura de toda llamada de todo servicio.

// Lockdown es el estado del interruptor.
type Lockdown struct {
	Active    bool
	Reason    string
	Actor     string
	ChangedAt time.Time
}

// IsLockedDown indica si el sistema está cerrado a nuevas emisiones.
// Fail-closed por construcción: el error se propaga y el llamador deniega.
func (s *Store) IsLockedDown(ctx context.Context) (bool, error) {
	const sql = `SELECT active FROM emergency_lockdown WHERE id = true`
	var activo bool
	err := s.DB().QueryRow(ctx, sql).Scan(&activo)
	return activo, errf("IsLockedDown", err)
}

// GetLockdown devuelve el estado completo (para informar por consola).
func (s *Store) GetLockdown(ctx context.Context) (Lockdown, error) {
	const sql = `SELECT active, reason, actor, changed_at FROM emergency_lockdown WHERE id = true`
	var l Lockdown
	err := s.DB().QueryRow(ctx, sql).Scan(&l.Active, &l.Reason, &l.Actor, &l.ChangedAt)
	return l, errf("GetLockdown", err)
}

// SetLockdown levanta o retira el bloqueo. El upsert incluye la fila por si una
// instalación llegara sin ella: durante una emergencia, «no existía la fila» no
// puede ser el motivo de que el interruptor no funcione.
func (s *Store) SetLockdown(ctx context.Context, q pg.Querier, active bool, actor, reason string) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `
		INSERT INTO emergency_lockdown (id, active, reason, actor, changed_at)
		VALUES (true, $1, $2, $3, now())
		ON CONFLICT (id) DO UPDATE
		SET active     = excluded.active,
		    reason     = excluded.reason,
		    actor      = excluded.actor,
		    changed_at = excluded.changed_at`
	_, err := q.Exec(ctx, sql, active, reason, actor)
	return errf("SetLockdown", err)
}
