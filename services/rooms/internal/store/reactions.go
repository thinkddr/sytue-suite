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

// AddReaction añade la reacción (emoji) del usuario a un mensaje. Idempotente.
// Devuelve el canal del mensaje (para fan-out). Exige acceso al canal.
func (s *Store) AddReaction(ctx context.Context, messageID uuid.UUID, subject, emoji string) (uuid.UUID, error) {
	t := tenancy.MustFrom(ctx)
	var channelID uuid.UUID
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		channelID, err = messageChannelTx(ctx, tx, t.Slug, messageID)
		if err != nil {
			return err
		}
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO reactions (tenant, message_id, subject, emoji)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (tenant, message_id, subject, emoji) DO NOTHING`,
			t.Slug, messageID, subject, emoji)
		return err
	})
	return channelID, err
}

// RemoveReaction retira la reacción del usuario de un mensaje.
func (s *Store) RemoveReaction(ctx context.Context, messageID uuid.UUID, subject, emoji string) (uuid.UUID, error) {
	t := tenancy.MustFrom(ctx)
	var channelID uuid.UUID
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		channelID, err = messageChannelTx(ctx, tx, t.Slug, messageID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`DELETE FROM reactions WHERE tenant=$1 AND message_id=$2 AND subject=$3 AND emoji=$4`,
			t.Slug, messageID, subject, emoji)
		return err
	})
	return channelID, err
}

// messageChannelTx devuelve el canal de un mensaje vivo dentro de una transacción.
func messageChannelTx(ctx context.Context, tx pgx.Tx, tenant string, messageID uuid.UUID) (uuid.UUID, error) {
	var channelID uuid.UUID
	err := tx.QueryRow(ctx,
		`SELECT channel_id FROM messages WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
		tenant, messageID).Scan(&channelID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, domain.ErrNotFound
	}
	return channelID, err
}

// attachReactions carga y agrega las reacciones de un lote de mensajes en una
// sola consulta (evita N+1), y las asigna a cada mensaje por ID.
func attachReactions(ctx context.Context, tx pgx.Tx, tenant string, msgs []domain.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(msgs))
	idx := make(map[uuid.UUID]int, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
		idx[m.ID] = i
	}
	rows, err := tx.Query(ctx,
		`SELECT message_id, emoji, subject FROM reactions
		 WHERE tenant=$1 AND message_id = ANY($2)
		 ORDER BY emoji, created_at`, tenant, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	// agg[messageID][emoji] -> índice en el slice de reacciones del mensaje.
	agg := make(map[uuid.UUID]map[string]int)
	for rows.Next() {
		var mid uuid.UUID
		var emoji, subject string
		if err := rows.Scan(&mid, &emoji, &subject); err != nil {
			return err
		}
		i, ok := idx[mid]
		if !ok {
			continue
		}
		byEmoji := agg[mid]
		if byEmoji == nil {
			byEmoji = make(map[string]int)
			agg[mid] = byEmoji
		}
		ri, ok := byEmoji[emoji]
		if !ok {
			msgs[i].Reactions = append(msgs[i].Reactions, domain.Reaction{Emoji: emoji})
			ri = len(msgs[i].Reactions) - 1
			byEmoji[emoji] = ri
		}
		msgs[i].Reactions[ri].Count++
		msgs[i].Reactions[ri].Subs = append(msgs[i].Reactions[ri].Subs, subject)
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// Read state
// ---------------------------------------------------------------------------

// upsertReadState fija el estado de lectura de un usuario en un canal hasta el
// mensaje dado (o NULL para "ahora").
func upsertReadState(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID, subject string, lastMessageID uuid.UUID) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO read_state (tenant, channel_id, subject, last_read_at, last_read_message_id)
		 VALUES ($1, $2, $3, now(), $4)
		 ON CONFLICT (tenant, channel_id, subject)
		 DO UPDATE SET last_read_at=now(), last_read_message_id=EXCLUDED.last_read_message_id`,
		tenant, channelID, subject, lastMessageID)
	return err
}

// MarkRead marca un canal como leído por el usuario hasta el mensaje indicado
// (o el más reciente si messageID es nil). Exige acceso al canal.
func (s *Store) MarkRead(ctx context.Context, channelID uuid.UUID, subject string, messageID *uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		mid := messageID
		if mid == nil {
			var last uuid.UUID
			err := tx.QueryRow(ctx,
				`SELECT id FROM messages
				 WHERE tenant=$1 AND channel_id=$2 AND deleted_at IS NULL
				 ORDER BY created_at DESC, id DESC LIMIT 1`, t.Slug, channelID).Scan(&last)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil {
				mid = &last
			}
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO read_state (tenant, channel_id, subject, last_read_at, last_read_message_id)
			 VALUES ($1, $2, $3, now(), $4)
			 ON CONFLICT (tenant, channel_id, subject)
			 DO UPDATE SET last_read_at=now(), last_read_message_id=EXCLUDED.last_read_message_id`,
			t.Slug, channelID, subject, mid)
		return err
	})
}

// MarkUnread deja el canal como NO leído a partir de un mensaje: fija el
// read_state del usuario a «justo antes» de ese mensaje (last_read_at =
// created_at del objetivo menos 1µs) y apunta last_read_message_id al mensaje
// anterior (el último que sí ha leído), o NULL si no hay ninguno. UnreadByChannel
// recalcula unread/mentions a partir de last_read_at, igual que MarkRead. Exige
// acceso al canal (derivado del mensaje).
func (s *Store) MarkUnread(ctx context.Context, messageID uuid.UUID, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var channelID uuid.UUID
		var createdAt time.Time
		err := tx.QueryRow(ctx,
			`SELECT channel_id, created_at FROM messages
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, messageID).Scan(&channelID, &createdAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		// Predecesor: el mensaje vivo inmediatamente anterior al objetivo (mismo
		// orden keyset que el feed). NULL si el objetivo es el primero del canal.
		var prev uuid.UUID
		var prevPtr *uuid.UUID
		err = tx.QueryRow(ctx,
			`SELECT id FROM messages
			 WHERE tenant=$1 AND channel_id=$2 AND deleted_at IS NULL AND created_at < $3
			 ORDER BY created_at DESC, id DESC LIMIT 1`,
			t.Slug, channelID, createdAt).Scan(&prev)
		if err == nil {
			prevPtr = &prev
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO read_state (tenant, channel_id, subject, last_read_at, last_read_message_id)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (tenant, channel_id, subject)
			 DO UPDATE SET last_read_at=EXCLUDED.last_read_at, last_read_message_id=EXCLUDED.last_read_message_id`,
			t.Slug, channelID, subject, createdAt.Add(-time.Microsecond), prevPtr)
		return err
	})
}

// DMReceipts devuelve el acuse de lectura de los OTROS miembros de un DM (Ola 16 ·
// «Visto»): su last_read_at, para que el cliente marque «Visto» en el último mensaje
// del usuario cuando el peer ya lo ha leído. SOLO para canales directos (DM 1:1 o de
// grupo): en un canal grande, mostrar receipts costaría un fan-out que Mattermost
// evita a propósito; aquí un canal no directo devuelve lista vacía (sin error).
//
// ESCALA: una sola consulta agregada (channel_members ⟕ read_state), SIN N+1: recoge
// el last_read_at de todos los peers de una vez. Exige ser miembro del canal.
func (s *Store) DMReceipts(ctx context.Context, channelID uuid.UUID, subject string) ([]domain.Receipt, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Receipt
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelMember(ctx, tx, t.Slug, channelID, subject); err != nil {
			return err
		}
		// Receipts solo en DMs: en canales grandes es coste innecesario (paridad Mattermost).
		if ch.Kind != domain.ChannelDirect {
			return nil
		}
		rows, err := tx.Query(ctx,
			`SELECT cm.subject, COALESCE(NULLIF(cm.name, ''), cm.subject), rs.last_read_at
			 FROM channel_members cm
			 LEFT JOIN read_state rs
			   ON rs.tenant=cm.tenant AND rs.channel_id=cm.channel_id AND rs.subject=cm.subject
			 WHERE cm.tenant=$1 AND cm.channel_id=$2 AND cm.subject <> $3
			 ORDER BY cm.name, cm.subject`,
			t.Slug, channelID, subject)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r domain.Receipt
			if err := rows.Scan(&r.Subject, &r.Name, &r.LastReadAt); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// UnreadByChannel devuelve, para cada canal accesible del team, el número de
// mensajes no leídos por el usuario (badge). Una sola consulta (sin N+1).
func (s *Store) UnreadByChannel(ctx context.Context, teamID uuid.UUID, subject string) ([]domain.ReadState, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.ReadState
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireTeamMember(ctx, tx, t.Slug, teamID, subject); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT c.id,
			       COALESCE(rs.last_read_at, 'epoch'::timestamptz) AS last_read_at,
			       rs.last_read_message_id,
			       (SELECT count(*) FROM messages m
			        WHERE m.tenant=c.tenant AND m.channel_id=c.id AND m.deleted_at IS NULL
			          AND m.author_sub <> $3
			          AND m.created_at > COALESCE(rs.last_read_at, 'epoch'::timestamptz)) AS unread,
			       (SELECT count(*) FROM messages m
			        WHERE m.tenant=c.tenant AND m.channel_id=c.id AND m.deleted_at IS NULL
			          AND m.author_sub <> $3
			          -- @usuario (índice GIN) O @channel/@here/@all (flag mentions_all,
			          -- índice parcial): una mención a todo el canal cuenta para cada
			          -- lector SIN insertar una fila por miembro (sin fan-out N).
			          AND (m.mentions @> ARRAY[$3]::text[] OR m.mentions_all)
			          AND m.created_at > COALESCE(rs.last_read_at, 'epoch'::timestamptz)) AS mentions
			FROM channels c
			LEFT JOIN read_state rs
			       ON rs.tenant=c.tenant AND rs.channel_id=c.id AND rs.subject=$3
			WHERE c.tenant=$1 AND c.team_id=$2 AND c.deleted_at IS NULL AND c.archived=false
			  AND (
			       c.kind='public'
			       OR EXISTS (SELECT 1 FROM channel_members m
			                  WHERE m.tenant=c.tenant AND m.channel_id=c.id AND m.subject=$3)
			  )`,
			t.Slug, teamID, subject)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var rs domain.ReadState
			if err := rows.Scan(&rs.ChannelID, &rs.LastReadAt, &rs.LastMessageID, &rs.Unread, &rs.Mentions); err != nil {
				return err
			}
			rs.Subject = subject
			out = append(out, rs)
		}
		return rows.Err()
	})
	return out, err
}
