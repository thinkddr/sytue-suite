// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Admins de PLATAFORMA (mig 0031). Alcance global, deliberadamente fuera del
// modelo de roles por organización: aquí no hay tenant que acotar, así que la
// tabla es una allowlist y la pregunta «¿tiene plataforma?» es un SELECT, no una
// deducción sobre pertenencias.
//
// La escritura la hacen el bootstrap del despliegue y la propia API de plataforma
// (un admin global puede nombrar a otro); ninguna API de organización la toca.

// IsPlatformAdmin indica si el usuario tiene alcance de plataforma. Fail-closed
// por construcción: un error de consulta se propaga y el llamador debe denegar,
// nunca interpretar «no lo sé» como «sí».
func (s *Store) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	const sql = `SELECT EXISTS (SELECT 1 FROM platform_admins WHERE user_id = $1)`
	var ok bool
	err := s.DB().QueryRow(ctx, sql, userID).Scan(&ok)
	return ok, errf("IsPlatformAdmin", err)
}

// UpsertPlatformAdmin concede alcance de plataforma. Idempotente: repetirlo
// refresca el motivo y la procedencia sin duplicar la concesión.
//
// grantedBy vacío ⇒ NULL: es el caso del bootstrap, donde no hay un humano
// anterior al primer admin. No se acepta el propio usuario como su concedente
// para que la procedencia no mienta.
func (s *Store) UpsertPlatformAdmin(ctx context.Context, q pg.Querier, userID, grantedBy, reason string) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `
		INSERT INTO platform_admins (user_id, granted_by, reason)
		VALUES ($1, nullif($2,'')::uuid, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET granted_by = excluded.granted_by,
		    reason     = excluded.reason,
		    updated_at = now()`
	_, err := q.Exec(ctx, sql, userID, grantedBy, reason)
	return errf("UpsertPlatformAdmin", err)
}
