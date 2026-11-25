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
//
// Lo guardado en una conversación de OTRA organización (ADR 0040) se escribió en la
// anfitriona, donde vive el mensaje: se lee allí también (enCadaAnfitrion) y se junta
// con lo propio por fecha, solo de las conversaciones donde sigue como externo.
func (s *Store) ListSaved(ctx context.Context, subject string, before *time.Time, limit int) ([]domain.Message, *time.Time, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	beforeTs := time.Now().Add(time.Hour) // primera página
	if before != nil {
		beforeTs = *before
	}
	var todos []conFecha[domain.Message]
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		todos, err = guardadosTx(ctx, tx, t.Slug, subject, beforeTs, limit, false)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	err = s.enCadaAnfitrion(ctx, subject, t.Slug, func(tx pgx.Tx, anfitrion string) (bool, error) {
		fuera, err := guardadosTx(ctx, tx, anfitrion, subject, beforeTs, limit, true)
		todos = append(todos, fuera...)
		return false, err
	})
	if err != nil {
		return nil, nil, err
	}
	msgs, next := paginaPorFecha(todos, limit)
	return msgs, next, nil
}

// guardadosTx lee una página de lo guardado por subject en UNA organización.
func guardadosTx(ctx context.Context, tx pgx.Tx, tenant, subject string, beforeTs time.Time, limit int, deFuera bool) ([]conFecha[domain.Message], error) {
	rows, err := tx.Query(ctx,
		`SELECT `+prefixed("m", messageCols)+`, sv.created_at
		 FROM message_saves sv
		 JOIN messages m ON m.tenant=sv.tenant AND m.id=sv.message_id
		 JOIN channels c ON c.tenant=sv.tenant AND c.id=sv.channel_id
		 WHERE sv.tenant=$1 AND sv.subject=$2
		   AND m.deleted_at IS NULL AND c.deleted_at IS NULL
		   AND sv.created_at < $3`+accesoAlCanal(deFuera)+filtroVaciado("m", "$2")+`
		 ORDER BY sv.created_at DESC, sv.message_id DESC
		 LIMIT $4`,
		tenant, subject, beforeTs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []conFecha[domain.Message]
	for rows.Next() {
		m, savedAt, err := scanSavedMessage(rows, tenant)
		if err != nil {
			return nil, err
		}
		out = append(out, conFecha[domain.Message]{v: m, fecha: savedAt, id: m.ID})
	}
	return out, rows.Err()
}
