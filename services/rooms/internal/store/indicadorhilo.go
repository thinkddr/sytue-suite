// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// enElFeed es el filtro de lo que pinta el feed de un canal: las raíces y las
// respuestas que su autor mandó «también al canal» mientras su raíz siga viva —borrar
// la raíz no borra las respuestas, y una difundida huérfana quedaba en el canal con un
// «Ver el hilo» que daba 404—. Implica el predicado del índice messages_channel_feed
// (mig. 0064), así que el keyset sigue siendo indexado.
const enElFeed = `(parent_id IS NULL OR (also_in_channel AND EXISTS (
	SELECT 1 FROM messages raiz
	 WHERE raiz.tenant = messages.tenant AND raiz.id = messages.parent_id AND raiz.deleted_at IS NULL)))`

// attachThreadInfo completa el indicador de hilo de las raíces con respuestas: quién
// respondió más recientemente (hasta domain.MaxThreadParticipants personas distintas)
// y cuándo fue la última respuesta. Una sola consulta agrupada para toda la página, sin
// N+1 y sin columna nueva: reply_count ya dice si hay hilo, y el índice de respuestas
// (messages_thread_feed) cubre el filtro por parent_id.
func attachThreadInfo(ctx context.Context, tx pgx.Tx, tenant string, msgs []domain.Message) error {
	idx := make(map[uuid.UUID]int)
	var ids []uuid.UUID
	for i, m := range msgs {
		if m.ParentID == nil && m.ReplyCount > 0 {
			ids = append(ids, m.ID)
			idx[m.ID] = i
		}
	}
	if len(ids) == 0 {
		return nil
	}
	// Una fila por (hilo, autor), de la respuesta más reciente a la más antigua: el
	// primero de cada hilo da la última respuesta, y los siguientes, los avatares. El
	// nombre es el de su respuesta más reciente, por si lo cambió entre medias.
	rows, err := tx.Query(ctx,
		`SELECT parent_id, author_sub,
		        (array_agg(author_name ORDER BY created_at DESC))[1],
		        max(created_at) AS ultima
		   FROM messages
		  WHERE tenant=$1 AND parent_id = ANY($2) AND deleted_at IS NULL
		  GROUP BY parent_id, author_sub
		  ORDER BY parent_id, ultima DESC, author_sub`, tenant, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var parent uuid.UUID
		var p domain.ThreadParticipant
		var ultima time.Time
		if err := rows.Scan(&parent, &p.Sub, &p.Name, &ultima); err != nil {
			return err
		}
		i, ok := idx[parent]
		if !ok {
			continue
		}
		m := &msgs[i]
		if m.LastReplyAt == nil {
			t := ultima
			m.LastReplyAt = &t
		}
		if len(m.ThreadParticipants) < domain.MaxThreadParticipants {
			m.ThreadParticipants = append(m.ThreadParticipants, p)
		}
	}
	return rows.Err()
}

// tambienAlCanal es la marca que se guarda: solo una respuesta puede ir también al
// canal; en una raíz no significa nada.
func (rec NewMessageRecord) tambienAlCanal() bool { return rec.ParentID != nil && rec.AlsoInChannel }

// completarFeed agrega a una página del feed lo que no son columnas del mensaje: las
// reacciones, el indicador de hilo, los acuses del visor y los votos de las tarjetas.
// Una consulta por cada cosa para toda la página (sin N+1), igual en el feed y en la
// ventana «alrededor de» un permalink.
func completarFeed(ctx context.Context, tx pgx.Tx, tenant, subject string, msgs []domain.Message) error {
	if err := attachReactions(ctx, tx, tenant, msgs); err != nil {
		return err
	}
	if err := attachThreadInfo(ctx, tx, tenant, msgs); err != nil {
		return err
	}
	if err := attachAcks(ctx, tx, tenant, subject, msgs); err != nil {
		return err
	}
	return attachCardVotes(ctx, tx, tenant, msgs)
}
