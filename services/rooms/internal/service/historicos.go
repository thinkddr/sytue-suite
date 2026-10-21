// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// AvisosHistoricos son los avisos de mensajes directos y menciones de la organización
// del contexto que la campana no llegó a guardar, con la hora de cada mensaje.
//
// Los destinatarios son los de entregarHubNotification (destinatariosCTE +
// avisables): el otro miembro de un DM y los @mencionados que son miembros, nunca el
// autor, con el nivel de aviso de cada canal. Y además, que ya fuera miembro cuando se
// escribió el mensaje: a quien entró después no se le enseña lo de antes. Fuera lo
// borrado, las tarjetas de sistema y lo que escriben integraciones, como en vivo.
func AvisosHistoricos(ctx context.Context, pool *pg.Pool, publicURL string) ([]hubnotify.Historico, error) {
	t := tenancy.MustFrom(ctx)
	var out []hubnotify.Historico
	err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT m.created_at, m.id, m.channel_id, m.author_sub, m.author_name, m.body,
			       cm.subject, cm.subject = ANY (m.mentions) AS mencionado
			FROM messages m
			JOIN channels ch ON ch.tenant = m.tenant AND ch.id = m.channel_id
			JOIN channel_members cm ON cm.tenant = m.tenant AND cm.channel_id = m.channel_id
			WHERE m.tenant = $1 AND m.deleted_at IS NULL AND ch.deleted_at IS NULL
			  AND m.type = $2
			  AND cm.subject <> m.author_sub AND cm.joined_at <= m.created_at
			  AND (ch.kind = 'direct' OR cm.subject = ANY (m.mentions))
			  AND ((cm.subject = ANY (m.mentions) AND cm.notify_level <> 'none')
			    OR (NOT cm.subject = ANY (m.mentions) AND cm.notify_level = 'all'))
			ORDER BY m.created_at, m.id`, t.Slug, domain.MessageTypeUser)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				h   hubnotify.Historico
				msg pushPayload
				r   domain.NotificationRecipient
			)
			if err := rows.Scan(&h.Fecha, &msg.ID, &msg.ChannelID, &msg.AuthorSub, &msg.AuthorName, &msg.Body,
				&r.Subject, &r.Mentioned); err != nil {
				return err
			}
			if domain.IsBotAuthor(msg.AuthorSub) {
				continue
			}
			h.Notification = avisoDeMensaje(publicURL, t.Slug, msg, r)
			h.Clave = "rooms:" + msg.ID.String()
			out = append(out, h)
		}
		return rows.Err()
	})
	return out, err
}
