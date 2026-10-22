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

// Quién ha visto un mensaje, y cuándo.
//
// EL DATO YA ESTABA. `read_state` guarda, por persona y canal, hasta dónde ha
// leído (`last_read_at`) desde la migración 0001 — es lo que pinta el contador de
// no leídos—. Lo que no había era forma de mirarlo al revés: dado un mensaje,
// quién ha llegado hasta él. Escribir quién leyó QUÉ, mensaje a mensaje, sería
// una fila por mensaje y por persona: en un canal de doscientos, cada mensaje
// costaría doscientas escrituras. Por eso todos los sistemas de chat guardan
// «leído hasta aquí» y responden esta pregunta comparando, que es lo que se hace
// aquí.
//
// LA COMPARACIÓN ES POR TIEMPO y no por identificador de mensaje: quien lee un
// mensaje POSTERIOR ha visto también los anteriores, y comparar identificadores
// obligaría a ordenar el histórico entero para saberlo.

// MessageReader es una persona que ha visto el mensaje.
type MessageReader struct {
	Subject string    `json:"subject"`
	Name    string    `json:"name,omitempty"`
	ReadAt  time.Time `json:"read_at"`
}

// MessageReaders devuelve quién ha visto `messageID`, más cuánta gente hay en el
// canal aparte del autor (el denominador de «visto por 3 de 12»).
//
// El AUTOR no cuenta: que hayas visto lo que acabas de escribir no es
// información. Quien pregunta debe tener acceso al canal, y por eso se comprueba
// aquí y no en el handler: es el único sitio por el que pasan todos los
// llamantes.
func (s *Store) MessageReaders(
	ctx context.Context, messageID uuid.UUID, viewer string,
) (lectores []MessageReader, total int, err error) {
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var channelID uuid.UUID
		var autor string
		var creado time.Time
		e := tx.QueryRow(ctx,
			`SELECT channel_id, subject, created_at FROM messages
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, messageID).Scan(&channelID, &autor, &creado)
		if errors.Is(e, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if e != nil {
			return e
		}
		ch, e := getChannelTx(ctx, tx, t.Slug, channelID)
		if e != nil {
			return e
		}
		if e := requireChannelAccess(ctx, tx, t.Slug, ch, viewer); e != nil {
			return e
		}

		// Quien está en el canal y ha leído hasta este mensaje o más allá. El JOIN
		// con `channel_members` no es decorativo: alguien que salió del canal
		// conserva su fila en `read_state`, y contarlo diría «visto por» gente que
		// ya no está.
		rows, e := tx.Query(ctx,
			`SELECT r.subject, COALESCE(m.name, ''), r.last_read_at
			   FROM read_state r
			   JOIN channel_members m
			     ON m.tenant = r.tenant AND m.channel_id = r.channel_id AND m.subject = r.subject
			  WHERE r.tenant = $1 AND r.channel_id = $2
			    AND r.subject <> $3
			    AND r.last_read_at >= $4
			  ORDER BY r.last_read_at`,
			t.Slug, channelID, autor, creado)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var l MessageReader
			if e := rows.Scan(&l.Subject, &l.Name, &l.ReadAt); e != nil {
				return e
			}
			lectores = append(lectores, l)
		}
		if e := rows.Err(); e != nil {
			return e
		}

		return tx.QueryRow(ctx,
			`SELECT count(*) FROM channel_members
			  WHERE tenant=$1 AND channel_id=$2 AND subject <> $3`,
			t.Slug, channelID, autor).Scan(&total)
	})
	return lectores, total, err
}
