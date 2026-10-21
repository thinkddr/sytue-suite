// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// AvisosHistoricos son los avisos de invitación de la organización del contexto que
// la campana no llegó a guardar, con la hora de la PRIMERA invitación a cada persona
// para cada evento: reenviarla al cambiar el evento no era otra invitación. Van a la
// dirección invitada, como en vivo (avisarInvitacion), solo si SIGUE invitada a un
// evento que sigue en pie y la invitación no se canceló, y nunca a quien organiza. Sin sesión, quien invita se
// nombra por la dirección del organizador.
func AvisosHistoricos(ctx context.Context, pool *pg.Pool, publicURL string) ([]hubnotify.Historico, error) {
	t := tenancy.MustFrom(ctx)
	var out []hubnotify.Historico
	err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT * FROM (
			    SELECT DISTINCT ON (i.uid, lower(i.recipient))
			           i.created_at, i.id::text, i.recipient, i.organizer, i.status, i.data
			    FROM invitations i
			    JOIN events e ON e.id = i.event_id AND e.deleted_at IS NULL
			    WHERE i.tenant = $1 AND i.method = 'REQUEST'
			      AND EXISTS (SELECT 1 FROM attendees a
			                  WHERE a.tenant = i.tenant AND a.event_id = e.id
			                    AND lower(a.email) = lower(i.recipient))
			    ORDER BY i.uid, lower(i.recipient), i.created_at, i.id
			) primera
			WHERE status <> 'cancelled'
			ORDER BY created_at`, t.Slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				h                         hubnotify.Historico
				id, para, org, est, datos string
			)
			if err := rows.Scan(&h.Fecha, &id, &para, &org, &est, &datos); err != nil {
				return err
			}
			// El título y la hora que RECIBIÓ, no los de ahora: el evento pudo cambiar
			// después, y eso no se lo dijo nadie.
			recibido, err := caldav.DecodeCalendarObject(datos, "")
			if err != nil {
				continue
			}
			ev := domain.Event{Summary: recibido.Master.Summary, DTStart: recibido.Master.DTStart}
			org = cleanAddr(org)
			if para = strings.TrimSpace(para); para == "" || strings.EqualFold(para, org) {
				continue
			}
			h.Notification = avisoDeInvitacion(publicURL, t.Slug, para, org, ev)
			h.Clave = "calendar:invitation:" + id
			out = append(out, h)
		}
		return rows.Err()
	})
	return out, err
}
