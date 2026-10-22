// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// PinMessage fija un mensaje en su canal. Permiso = acceso al canal (cualquier
// miembro puede fijar, convención Mattermost por defecto). Idempotente. Devuelve
// el mensaje fijado (para el fan-out del evento y el panel de fijados).
func (s *Store) PinMessage(ctx context.Context, messageID uuid.UUID, subject string) (domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	var m domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		m, err = getMessageTx(ctx, tx, t.Slug, messageID)
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
			`INSERT INTO message_pins (tenant, message_id, channel_id, pinned_by)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (tenant, message_id) DO NOTHING`,
			t.Slug, messageID, m.ChannelID, subject)
		return err
	})
	return m, err
}

// UnpinMessage desfija un mensaje. Permiso = acceso al canal. El canal se lee de
// la propia fila de fijado (funciona aunque el mensaje ya esté borrado). Devuelve
// el canal (para el fan-out del evento de desfijado).
func (s *Store) UnpinMessage(ctx context.Context, messageID uuid.UUID, subject string) (uuid.UUID, error) {
	t := tenancy.MustFrom(ctx)
	var channelID uuid.UUID
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT channel_id FROM message_pins WHERE tenant=$1 AND message_id=$2`,
			t.Slug, messageID).Scan(&channelID)
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
		_, err = tx.Exec(ctx,
			`DELETE FROM message_pins WHERE tenant=$1 AND message_id=$2`, t.Slug, messageID)
		return err
	})
	return channelID, err
}

// ListPins devuelve los mensajes fijados de un canal (más reciente primero),
// vía el índice message_pins_by_channel. Los fijados de un canal son un conjunto
// acotado; se cap­an a 200 sin keyset.
//
// ponytail: sin reacciones (el panel de fijados muestra cuerpo/autor/hora, como
// ThreadPanel); adjuntar reacciones si el panel las llega a mostrar.
func (s *Store) ListPins(ctx context.Context, channelID uuid.UUID, subject string) ([]domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	var msgs []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		// Lo que el usuario borró para sí en un directo tampoco sale fijado (mig 0058).
		corte, err := vaciadoAntesDe(ctx, tx, t.Slug, channelID, subject)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT `+prefixed("m", messageCols)+` FROM messages m
			 JOIN message_pins p ON p.tenant=m.tenant AND p.message_id=m.id
			 WHERE m.tenant=$1 AND p.channel_id=$2 AND m.deleted_at IS NULL
			   AND ($3::timestamptz IS NULL OR m.created_at > $3)
			 ORDER BY p.pinned_at DESC, p.message_id DESC
			 LIMIT 200`,
			t.Slug, channelID, corte)
		if err != nil {
			return err
		}
		msgs, err = collectMessages(rows, t.Slug)
		return err
	})
	return msgs, err
}
