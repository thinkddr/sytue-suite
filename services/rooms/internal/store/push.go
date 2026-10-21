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
// caminos de aviso (Web Push y APNs) y deben decidir exactamente lo mismo: si uno se
// quedara atrás, el mismo mensaje avisaría en el navegador y no en el móvil.
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
const avisables = ` WHERE (r.mentioned AND r.notify_level <> 'none')
			    OR (NOT r.mentioned AND r.notify_level = 'all')`

// PushTargetsForMessage devuelve las suscripciones push a las que avisar de un mensaje
// nuevo (Ola 18): los usuarios @mencionados y —si el canal es un DM— el otro miembro,
// excluyendo al autor. Respeta el nivel de aviso del canal (Ola 13) BARATO en SQL: a un
// mencionado se le avisa salvo notify_level='none' (silenciado); a un miembro de DM solo
// con notify_level='all' (por defecto), igual que la política del cliente. Una sola
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
			 JOIN push_subscriptions ps ON ps.tenant = $1 AND ps.subject = r.subject`+avisables,
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

// SaveAPNSDevice hace UPSERT del token de dispositivo de la app nativa de iOS. La clave es
// (tenant, token): el mismo aparato puede cambiar de dueño al reinstalar, y entonces manda
// el último que registró. Aislado por tenant vía RLS (mig. 0032).
func (s *Store) SaveAPNSDevice(ctx context.Context, subject, token string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO push_apns (tenant, subject, token)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (tenant, token) DO UPDATE SET subject = EXCLUDED.subject`,
			t.Slug, subject, token)
		return err
	})
}

// DeleteAPNSDevice borra un token por (subject, token). La usan el «olvidar este aparato»
// del usuario y la limpieza del enviador cuando Apple reporta el token caduco (410).
// Idempotente.
func (s *Store) DeleteAPNSDevice(ctx context.Context, subject, token string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`DELETE FROM push_apns WHERE tenant = $1 AND subject = $2 AND token = $3`,
			t.Slug, subject, token)
		return err
	})
}

// APNSTargetsForMessage devuelve los dispositivos de iOS a los que despertar por un
// mensaje nuevo. Usa EL MISMO CTE de destinatarios que el Web Push, así que los dos
// caminos avisan a la misma gente.
func (s *Store) APNSTargetsForMessage(ctx context.Context, channelID uuid.UUID, authorSub string, mentions []string) ([]domain.APNSTarget, error) {
	t := tenancy.MustFrom(ctx)
	if mentions == nil {
		mentions = []string{}
	}
	var out []domain.APNSTarget
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			destinatariosCTE+`
			 SELECT r.subject, pa.token
			 FROM recipients r
			 JOIN push_apns pa ON pa.tenant = $1 AND pa.subject = r.subject`+avisables,
			t.Slug, channelID, authorSub, mentions)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var at domain.APNSTarget
			if err := rows.Scan(&at.Subject, &at.Token); err != nil {
				return err
			}
			out = append(out, at)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return sinQuienesBloquean(ctx, s, authorSub, out, func(x domain.APNSTarget) string { return x.Subject })
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
