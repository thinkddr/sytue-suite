// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// rootForFollow valida que rootID es un mensaje RAÍZ (parent_id IS NULL) vivo y
// accesible por el usuario, y devuelve su canal. Base de seguir/dejar de seguir un
// hilo: nunca se sigue una respuesta ni un hilo de un canal al que no tienes acceso.
func rootForFollow(ctx context.Context, tx pgx.Tx, tenant string, rootID uuid.UUID, subject string) error {
	var channelID uuid.UUID
	var parentID *uuid.UUID
	err := tx.QueryRow(ctx,
		`SELECT channel_id, parent_id FROM messages
		 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, tenant, rootID).Scan(&channelID, &parentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if parentID != nil {
		return domain.ErrInvalidInput // no es un mensaje raíz
	}
	ch, err := getChannelTx(ctx, tx, tenant, channelID)
	if err != nil {
		return err
	}
	return requireChannelAccess(ctx, tx, tenant, ch, subject)
}

// FollowThread hace que el usuario siga un hilo (mensaje raíz). Idempotente. Valida
// que rootID es un mensaje raíz accesible por el usuario.
func (s *Store) FollowThread(ctx context.Context, rootID uuid.UUID, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := rootForFollow(ctx, tx, t.Slug, rootID, subject); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO thread_follows (tenant, root_id, subject)
			 VALUES ($1, $2, $3) ON CONFLICT (tenant, root_id, subject) DO NOTHING`,
			t.Slug, rootID, subject)
		return err
	})
}

// UnfollowThread deja de seguir un hilo. ErrNotFound si no lo seguía.
func (s *Store) UnfollowThread(ctx context.Context, rootID uuid.UUID, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`DELETE FROM thread_follows WHERE tenant=$1 AND root_id=$2 AND subject=$3`,
			t.Slug, rootID, subject)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// IsFollowingThread indica si el usuario sigue el hilo (para el botón Seguir/
// Siguiendo del panel de hilo). Consulta puntual por PK.
func (s *Store) IsFollowingThread(ctx context.Context, rootID uuid.UUID, subject string) (bool, error) {
	t := tenancy.MustFrom(ctx)
	var following bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM thread_follows
			              WHERE tenant=$1 AND root_id=$2 AND subject=$3)`,
			t.Slug, rootID, subject).Scan(&following)
	})
	return following, err
}

// autoFollowOnReply mantiene los seguimientos de hilo al publicar una respuesta, todo
// dentro de la transacción de CreateMessage:
//  1. bombea last_activity de los SEGUIDORES actuales del hilo (para que suba en su
//     vista «Hilos»): fan-out acotado a los participantes, no a N miembros del canal;
//  2. incorpora al que responde y lo marca al día (last_viewed_at=now());
//  3. incorpora al autor de la raíz (si aún no seguía), anclando su last_viewed_at al
//     created_at de la raíz para que ESTA respuesta le cuente como no leída.
func autoFollowOnReply(ctx context.Context, tx pgx.Tx, tenant string, rootID uuid.UUID, rootCreatedAt time.Time, replier, rootAuthor string) error {
	if _, err := tx.Exec(ctx,
		`UPDATE thread_follows SET last_activity=now() WHERE tenant=$1 AND root_id=$2`,
		tenant, rootID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO thread_follows (tenant, root_id, subject, last_viewed_at, last_activity)
		 VALUES ($1, $2, $3, now(), now())
		 ON CONFLICT (tenant, root_id, subject)
		 DO UPDATE SET last_viewed_at=now(), last_activity=now()`,
		tenant, rootID, replier); err != nil {
		return err
	}
	if rootAuthor != "" && rootAuthor != replier {
		if _, err := tx.Exec(ctx,
			`INSERT INTO thread_follows (tenant, root_id, subject, last_viewed_at, last_activity)
			 VALUES ($1, $2, $3, $4, now())
			 ON CONFLICT (tenant, root_id, subject) DO NOTHING`,
			tenant, rootID, rootAuthor, rootCreatedAt); err != nil {
			return err
		}
	}
	return nil
}

// markThreadViewed pone last_viewed_at=now() para el seguimiento del usuario (no-op si
// no sigue el hilo). Lo invoca ListThread al abrir un hilo: marca el hilo como visto y
// pone a cero su contador de respuestas no leídas en la vista global.
func markThreadViewed(ctx context.Context, tx pgx.Tx, tenant string, rootID uuid.UUID, subject string) error {
	_, err := tx.Exec(ctx,
		`UPDATE thread_follows SET last_viewed_at=now()
		 WHERE tenant=$1 AND root_id=$2 AND subject=$3`,
		tenant, rootID, subject)
	return err
}

// ListFollowedThreads devuelve una página de los hilos que el usuario SIGUE, ordenados
// por última actividad descendente (keyset por `before` sobre last_activity). Cada hilo
// trae su mensaje raíz (con ReplyCount = total de respuestas) y el nº de respuestas NO
// leídas (created_at > last_viewed_at, de otros autores) por subconsulta indexada —
// UNA sola consulta, sin N+1 (mismo patrón que UnreadByChannel). Reaplica el filtro de
// acceso al canal (público, o miembro) como ListSaved: no revela un hilo de un canal
// privado del que ya no eres miembro. Devuelve el cursor de la siguiente página o nil.
func (s *Store) ListFollowedThreads(ctx context.Context, subject string, before *time.Time, limit int) ([]domain.ThreadSummary, *time.Time, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	beforeTs := time.Now().Add(time.Hour) // primera página
	if before != nil {
		beforeTs = *before
	}
	var out []domain.ThreadSummary
	var next *time.Time
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+prefixed("m", messageCols)+`, f.last_activity,
			        (SELECT count(*) FROM messages r
			         WHERE r.tenant=f.tenant AND r.parent_id=f.root_id AND r.deleted_at IS NULL
			           AND r.author_sub <> f.subject
			           AND r.created_at > f.last_viewed_at) AS unread_replies
			 FROM thread_follows f
			 JOIN messages m ON m.tenant=f.tenant AND m.id=f.root_id AND m.deleted_at IS NULL
			 JOIN channels c ON c.tenant=m.tenant AND c.id=m.channel_id AND c.deleted_at IS NULL
			 WHERE f.tenant=$1 AND f.subject=$2 AND f.last_activity < $3
			   AND (
			        c.kind='public'
			        OR EXISTS (SELECT 1 FROM channel_members cm
			                   WHERE cm.tenant=c.tenant AND cm.channel_id=c.id AND cm.subject=$2)
			   )
			 ORDER BY f.last_activity DESC, f.root_id DESC
			 LIMIT $4`,
			t.Slug, subject, beforeTs, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		var lastActivity time.Time
		for rows.Next() {
			var ts domain.ThreadSummary
			m, err := scanMessage(rows, t.Slug, &ts.LastActivity, &ts.UnreadReplies)
			if err != nil {
				return err
			}
			ts.Root = m
			out = append(out, ts)
			lastActivity = ts.LastActivity
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) == limit {
			next = &lastActivity
		}
		return nil
	})
	return out, next, err
}
