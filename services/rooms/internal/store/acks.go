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

// AckMessage registra el acuse de recibo del usuario sobre un mensaje que pide
// acuse (requires_ack). Idempotente (PK). Exige acceso al canal. Devuelve el canal
// (para el fan-out) y el nuevo total de acuses. ErrInvalidInput si el mensaje no
// solicita acuse.
func (s *Store) AckMessage(ctx context.Context, messageID uuid.UUID, subject string) (uuid.UUID, int, error) {
	t := tenancy.MustFrom(ctx)
	var channelID uuid.UUID
	var count int
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var requiresAck bool
		err := tx.QueryRow(ctx,
			`SELECT channel_id, requires_ack FROM messages
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, messageID).Scan(&channelID, &requiresAck)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if !requiresAck {
			return domain.ErrInvalidInput // el mensaje no pide acuse
		}
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO message_acks (tenant, message_id, subject)
			 VALUES ($1, $2, $3) ON CONFLICT (tenant, message_id, subject) DO NOTHING`,
			t.Slug, messageID, subject); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM message_acks WHERE tenant=$1 AND message_id=$2`,
			t.Slug, messageID).Scan(&count)
	})
	return channelID, count, err
}

// attachAcks agrega, en UNA sola consulta (sin N+1), el nº de acuses de cada
// mensaje del lote que pide acuse (AckCount) y si el VISOR ya confirmó (Acked). Los
// mensajes sin requires_ack no se consultan (nadie los confirma): la lista ANY($2)
// se limita a los que lo piden. Mismo patrón agregado que attachReactions.
func attachAcks(ctx context.Context, tx pgx.Tx, tenant, viewer string, msgs []domain.Message) error {
	ids := make([]uuid.UUID, 0, len(msgs))
	idx := make(map[uuid.UUID]int, len(msgs))
	for i, m := range msgs {
		if !m.RequiresAck {
			continue
		}
		ids = append(ids, m.ID)
		idx[m.ID] = i
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx,
		`SELECT message_id, subject FROM message_acks
		 WHERE tenant=$1 AND message_id = ANY($2)`, tenant, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var mid uuid.UUID
		var subject string
		if err := rows.Scan(&mid, &subject); err != nil {
			return err
		}
		i, ok := idx[mid]
		if !ok {
			continue
		}
		msgs[i].AckCount++
		if subject == viewer {
			msgs[i].Acked = true
		}
	}
	return rows.Err()
}
