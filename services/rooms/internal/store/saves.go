// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// SaveMessage marca un mensaje como guardado por el usuario (por-usuario, no por
// canal). Idempotente. Exige acceso al canal del mensaje; denormaliza channel_id
// para reaplicar ese filtro de acceso al listar.
func (s *Store) SaveMessage(ctx context.Context, messageID uuid.UUID, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		m, err := getMessageTx(ctx, tx, t.Slug, messageID)
		if err != nil {
			return err
		}
		ch, err := getChannelTx(ctx, tx, t.Slug, m.ChannelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO message_saves (tenant, subject, message_id, channel_id)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (tenant, subject, message_id) DO NOTHING`,
			t.Slug, subject, messageID, m.ChannelID)
		return err
	})
}

// UnsaveMessage retira un mensaje de los guardados del usuario.
func (s *Store) UnsaveMessage(ctx context.Context, messageID uuid.UUID, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`DELETE FROM message_saves WHERE tenant=$1 AND subject=$2 AND message_id=$3`,
			t.Slug, subject, messageID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// scanSavedMessage escanea un mensaje más la marca de guardado (cursor keyset):
// la consulta añade una columna (message_saves.created_at) al final de messageCols.
func scanSavedMessage(row pgx.Row, tenant string) (domain.Message, time.Time, error) {
	var savedAt time.Time
	m, err := scanMessage(row, tenant, &savedAt)
	if err != nil {
		return domain.Message{}, time.Time{}, err
	}
	return m, savedAt, nil
}

// ListSaved devuelve una página de los mensajes que el usuario guardó, cross-canal,
// ordenados por fecha de guardado descendente (keyset por `before`). Reaplica el
// filtro de acceso al canal (público, o miembro) como SearchMessages: no revela un
// mensaje de un canal privado al que el usuario ya no pertenece. Devuelve el cursor
// de la siguiente página (la fecha de guardado del último elemento) o nil si no hay
// más.
func (s *Store) ListSaved(ctx context.Context, subject string, before *time.Time, limit int) ([]domain.Message, *time.Time, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	beforeTs := time.Now().Add(time.Hour) // primera página
	if before != nil {
		beforeTs = *before
	}
	var msgs []domain.Message
	var next *time.Time
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+prefixed("m", messageCols)+`, sv.created_at
			 FROM message_saves sv
			 JOIN messages m ON m.tenant=sv.tenant AND m.id=sv.message_id
			 JOIN channels c ON c.tenant=sv.tenant AND c.id=sv.channel_id
			 WHERE sv.tenant=$1 AND sv.subject=$2
			   AND m.deleted_at IS NULL AND c.deleted_at IS NULL
			   AND sv.created_at < $3
			   AND (
			        c.kind='public'
			        OR EXISTS (SELECT 1 FROM channel_members cm
			                   WHERE cm.tenant=c.tenant AND cm.channel_id=c.id AND cm.subject=$2)
			   )`+filtroVaciado("m", "$2")+`
			 ORDER BY sv.created_at DESC, sv.message_id DESC
			 LIMIT $4`,
			t.Slug, subject, beforeTs, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		var lastSavedAt time.Time
		for rows.Next() {
			m, savedAt, err := scanSavedMessage(rows, t.Slug)
			if err != nil {
				return err
			}
			msgs = append(msgs, m)
			lastSavedAt = savedAt
		}
		if err := rows.Err(); err != nil {
			return err
		}
		// Página completa: hay potencialmente más; expón el cursor keyset.
		if len(msgs) == limit {
			next = &lastSavedAt
		}
		return nil
	})
	return msgs, next, err
}
