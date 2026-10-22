// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// Heartbeat registra/renueva la presencia de un usuario (upsert). El estado
// efímero vive en PostgreSQL (sin Redis/Valkey); la obsolescencia se calcula al
// leer comparando last_seen con la TTL de presencia.
func (s *Store) Heartbeat(ctx context.Context, subject string, state domain.PresenceState, status string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO presence (tenant, subject, state, status, last_seen)
			 VALUES ($1, $2, $3, $4, now())
			 ON CONFLICT (tenant, subject)
			 DO UPDATE SET state = EXCLUDED.state, status = EXCLUDED.status, last_seen = now()`,
			t.Slug, subject, string(state), status)
		return err
	})
}

// SetOffline marca a un usuario como offline explícitamente (logout/cierre).
func (s *Store) SetOffline(ctx context.Context, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE presence SET state = 'offline', last_seen = now()
			 WHERE tenant=$1 AND subject=$2`, t.Slug, subject)
		return err
	})
}

// GetPresence devuelve la presencia de un conjunto de usuarios, derivando
// offline para quienes superen la TTL. Los subjects no encontrados se omiten.
//
// La caducidad la decide PostgreSQL (`last_seen < now() - ttl`), NO Go. `last_seen`
// lo escribe `now()` del servidor, así que compararlo contra un `time.Now()` del
// proceso mezcla DOS RELOJES: si el de la base va por delante, el heartbeat parece
// del futuro y la presencia no caduca nunca. En producción la app y la base están
// en máquinas distintas, y en los tests PostgreSQL corre dentro de la VM de Colima,
// cuyo reloj se desvía del host cientos de milisegundos.
func (s *Store) GetPresence(ctx context.Context, subjects []string, ttl time.Duration) ([]domain.Presence, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Presence
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT subject, state, status, last_seen,
			        last_seen < now() - make_interval(secs => $3) AS stale
			 FROM presence WHERE tenant=$1 AND subject = ANY($2)`,
			t.Slug, subjects, ttl.Seconds())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p domain.Presence
			var state string
			var stale bool
			if err := rows.Scan(&p.Subject, &state, &p.Status, &p.LastSeen, &stale); err != nil {
				return err
			}
			p.State = domain.PresenceState(state)
			if stale {
				p.State = domain.StateOffline
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// PurgeStalePresence elimina filas de presencia muy antiguas (limpieza). Opera
// sobre todos los tenants (mantenimiento); devuelve filas eliminadas.
func (s *Store) PurgeStalePresence(ctx context.Context, before time.Time) (int64, error) {
	var affected int64
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := enableMaintenance(ctx, tx); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM presence WHERE last_seen < $1`, before)
		if err != nil {
			return err
		}
		affected = tag.RowsAffected()
		return nil
	})
	return affected, err
}
