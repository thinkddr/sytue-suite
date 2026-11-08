// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// SavePushSubscription hace UPSERT de una suscripción Web Push del usuario (Ola 18). La
// clave es (tenant, endpoint): re-suscribir el mismo endpoint (mismo o distinto usuario en
// ese navegador) actualiza el dueño y las claves. Estado PER-USUARIO, aislado por tenant
// vía RLS (mig. 0016).
func (s *Store) SavePushSubscription(ctx context.Context, subject string, sub domain.PushSubscription) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO push_subscriptions (tenant, subject, endpoint, p256dh, auth_key)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (tenant, endpoint)
			 DO UPDATE SET subject = EXCLUDED.subject, p256dh = EXCLUDED.p256dh, auth_key = EXCLUDED.auth_key`,
			t.Slug, subject, sub.Endpoint, sub.P256dh, sub.Auth)
		return err
	})
}

// DeletePushSubscription borra una suscripción por (subject, endpoint). La usan tanto el
// desuscribir del usuario como la limpieza del enviador cuando el servicio de push reporta
// la suscripción caduca (404/410). Idempotente (borrar lo inexistente no es error).
func (s *Store) DeletePushSubscription(ctx context.Context, subject, endpoint string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`DELETE FROM push_subscriptions WHERE tenant = $1 AND subject = $2 AND endpoint = $3`,
			t.Slug, subject, endpoint)
		return err
	})
}

// destinatariosCTE resuelve a quién hay que avisar de un mensaje nuevo: los @mencionados
// y —si el canal es un DM— el otro miembro, excluyendo al autor, con el nivel de aviso del
// canal (Ola 13) aplicado BARATO en SQL. Vive en una constante porque lo usan los DOS
// caminos de aviso (Web Push y la bandeja del Hub) y deben decidir exactamente lo
// mismo. El despertar de iOS no lo usa: sigue al contador del widget
// (DespertablesForMessage).
// Parámetros: $1 tenant, $2 canal, $3 autor, $4 menciones.
const destinatariosCTE = `WITH ch AS (
			     SELECT kind FROM channels WHERE tenant = $1 AND id = $2
			 ),
			 recipients AS (
			     SELECT subject, notify_level, bool_or(mentioned) AS mentioned
			     FROM (
			         SELECT cm.subject, cm.notify_level, false AS mentioned
			         FROM channel_members cm, ch
			         WHERE cm.tenant = $1 AND cm.channel_id = $2
			           AND ch.kind = 'direct' AND cm.subject <> $3
			         UNION ALL
			         SELECT cm.subject, cm.notify_level, true AS mentioned
			         FROM channel_members cm
			         WHERE cm.tenant = $1 AND cm.channel_id = $2
			           AND cm.subject = ANY($4) AND cm.subject <> $3
			     ) u
			     GROUP BY subject, notify_level
			 )`

// avisables filtra los destinatarios por su nivel de aviso: a un mencionado se le avisa
// salvo que tenga el canal silenciado; a un miembro de DM solo con el nivel por defecto.
// Va entre paréntesis para que lo que se le añada con AND valga para las dos ramas.
const avisables = ` WHERE ((r.mentioned AND r.notify_level <> 'none')
			    OR (NOT r.mentioned AND r.notify_level = 'all'))`

// sinPausa deja fuera a quien tiene «Pausar avisos» en curso (mig. 0061). A diferencia
// de la ventana «No molestar», `paused_until` es un instante absoluto y el servidor lo
// compara sin conocer la zona horaria de nadie. Solo la llevan las alertas AL SISTEMA
// (Web Push); el despertar de iOS no suena y no la mira. El del Hub no se filtra aquí:
// es también la entrada de su bandeja, que no debe perderse por una pausa; callar su
// notificación es cosa del Hub.
const sinPausa = `
			   AND NOT EXISTS (SELECT 1 FROM user_notify_prefs np
			       WHERE np.tenant = $1 AND np.subject = r.subject AND np.paused_until > now())`

// PushTargetsForMessage devuelve las suscripciones push a las que avisar de un mensaje
// nuevo (Ola 18): los usuarios @mencionados y —si el canal es un DM— el otro miembro,
// excluyendo al autor. Respeta el nivel de aviso del canal (Ola 13) BARATO en SQL: a un
// mencionado se le avisa salvo notify_level='none' (silenciado); a un miembro de DM solo
// con notify_level='all' (por defecto), igual que la política del cliente; y a nadie
// con «Pausar avisos» en curso (sinPausa). Una sola
// consulta indexada (channel_members PK + push_subscriptions_subject_idx), sin N+1.
func (s *Store) PushTargetsForMessage(ctx context.Context, channelID uuid.UUID, authorSub string, mentions []string) ([]domain.PushTarget, error) {
	t := tenancy.MustFrom(ctx)
	if mentions == nil {
		mentions = []string{}
	}
	var out []domain.PushTarget
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			destinatariosCTE+`
			 SELECT r.subject, ps.endpoint, ps.p256dh, ps.auth_key
			 FROM recipients r
			 JOIN push_subscriptions ps ON ps.tenant = $1 AND ps.subject = r.subject`+avisables+sinPausa,
			t.Slug, channelID, authorSub, mentions)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pt domain.PushTarget
			if err := rows.Scan(&pt.Subject, &pt.Endpoint, &pt.P256dh, &pt.Auth); err != nil {
				return err
			}
			out = append(out, pt)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return sinQuienesBloquean(ctx, s, authorSub, out, func(x domain.PushTarget) string { return x.Subject })
}

// maxDespertables acota lo que se lee por mensaje: es el pushMaxTargets del servicio,
// que despierta como mucho a esos.
const maxDespertables = 50

// DespertablesForMessage devuelve a quién despertar la app de iOS por un mensaje nuevo:
// a quien le cambia el contador del widget de Rooms (api.PendientesRooms, la regla de
// la lista): los miembros del canal salvo el autor con el nivel por defecto (`all`), y
// con «menciones» solo los mencionados (todos con @channel, `mentionsAll`, que el
// contador cuenta como mención de cada uno). Silenciado (`none`) no suma y no despierta.
//
// NO es el criterio del Web Push, y a propósito: aquél es una ALERTA y solo va a los
// mencionados y al otro miembro de un DM; esto es un aviso VACÍO para que el contador
// del widget no se quede atrás, y el contador cuenta todo lo no leído de un canal con
// el nivel por defecto. Por lo mismo no mira «Pausar avisos» (sinPausa): la pausa calla
// alertas, y aquí no suena nada; ni los bloqueos (sinQuienesBloquean), porque lo no
// leído de un bloqueado también suma en el contador (Store.Unread no los mira) y el
// despertar no enseña nada de él. Primero los mencionados, para que el tope del
// servicio nunca deje fuera una mención; sin mirar si tienen aparato, que eso lo sabe
// el Hub (ADR 0039).
func (s *Store) DespertablesForMessage(ctx context.Context, channelID uuid.UUID, authorSub string, mentions []string, mentionsAll bool) ([]string, error) {
	t := tenancy.MustFrom(ctx)
	if mentions == nil {
		mentions = []string{}
	}
	var out []string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT cm.subject
			 FROM channel_members cm
			 WHERE cm.tenant = $1 AND cm.channel_id = $2 AND cm.subject <> $3
			   AND (cm.notify_level = 'all'
			        OR (cm.notify_level = 'mentions' AND ($6 OR cm.subject = ANY($4))))
			 ORDER BY (cm.subject = ANY($4)) DESC, cm.subject
			 LIMIT $5`,
			t.Slug, channelID, authorSub, mentions, maxDespertables, mentionsAll)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	return out, err
}

// NotificationRecipientsForMessage devuelve los sujetos destinatarios a los que avisar de un mensaje
// nuevo (Ola 18/Hub): los usuarios @mencionados y —si el canal es un DM— el otro miembro,
// excluyendo al autor, aplicando las preferencias de notificación de canal.
func (s *Store) NotificationRecipientsForMessage(ctx context.Context, channelID uuid.UUID, authorSub string, mentions []string) ([]domain.NotificationRecipient, error) {
	t := tenancy.MustFrom(ctx)
	if mentions == nil {
		mentions = []string{}
	}
	var out []domain.NotificationRecipient
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			destinatariosCTE+`
			 SELECT r.subject, r.mentioned
			 FROM recipients r`+avisables,
			t.Slug, channelID, authorSub, mentions)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var nr domain.NotificationRecipient
			if err := rows.Scan(&nr.Subject, &nr.Mentioned); err != nil {
				return err
			}
			out = append(out, nr)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return sinQuienesBloquean(ctx, s, authorSub, out, func(x domain.NotificationRecipient) string { return x.Subject })
}
