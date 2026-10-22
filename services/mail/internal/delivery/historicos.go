// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// AvisosHistoricos son los avisos de «correo nuevo» de la organización del contexto
// que la campana no llegó a guardar: los mismos que deja la entrega (avisoDeLlegada),
// con la hora en que entró cada correo.
//
// Qué es «recibido» lo dice el outbox: publishDelivered apunta un
// `mail.message.received` en cada entrega SMTP, y nada más lo hace, así que ni lo
// importado ni lo enviado cuenta. Fuera lo que entró en Spam y lo que está ahora en
// Spam, igual que en vivo, y lo borrado. El destinatario es el dueño de la cuenta
// que lo recibió, el mismo que en vivo.
func AvisosHistoricos(ctx context.Context, pool *pg.Pool, publicURL string) ([]hubnotify.Historico, error) {
	t := tenancy.MustFrom(ctx)
	var out []hubnotify.Historico
	err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT e.created_at, m.id, m.thread_id, m.subject, m.from_addr, m.from_name, a.subject
			FROM events_outbox e
			JOIN messages m ON m.id = (e.payload->>'message_id')::uuid
			                AND m.account_id = (e.payload->>'account_id')::uuid
			JOIN accounts a ON a.id = m.account_id
			JOIN mailboxes entrega ON entrega.id = (e.payload->>'mailbox_id')::uuid
			JOIN mailboxes ahora ON ahora.id = m.mailbox_id
			WHERE e.tenant = $1 AND e.type = 'mail.message.received'
			  AND m.deleted_at IS NULL AND a.subject <> ''
			  AND entrega.role <> $2 AND ahora.role <> $2
			ORDER BY e.created_at`, t.Slug, string(domain.RoleJunk))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				h     hubnotify.Historico
				m     domain.Message
				hilo  *uuid.UUID
				dueno string
			)
			if err := rows.Scan(&h.Fecha, &m.ID, &hilo, &m.Subject, &m.FromAddr, &m.FromName, &dueno); err != nil {
				return err
			}
			if hilo != nil {
				m.ThreadID = *hilo
			}
			h.Notification = avisoDeLlegada(publicURL, t.Slug, dueno, m)
			h.Clave = "mail:" + m.ID.String()
			out = append(out, h)
		}
		return rows.Err()
	})
	return out, err
}
