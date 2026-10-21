// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Conversaciones con personas de OTRA organización (ADR 0040), vistas desde la
// organización de quien viene de fuera.
//
// La conversación vive en la organización que la abrió (la ANFITRIONA) y la persona
// de fuera es una fila `externo` de channel_members allí. Su token trae SU
// organización, así que todo lo de este fichero empieza igual: averiguar en qué
// organizaciones está como externa. Esa es la ÚNICA consulta entre organizaciones
// y va por la política de mantenimiento, que en channel_members solo deja ver las
// filas `externo` (mig 0036). Lo demás —el canal, sus mensajes, sus no leídos— se
// lee dentro de cada organización anfitriona con su RLS de siempre.

// tenantsAnfitriones devuelve las organizaciones, distintas de la propia, donde
// subject está como persona de fuera. En régimen normal viene vacía, y es una
// consulta sobre un índice parcial.
func (s *Store) tenantsAnfitriones(ctx context.Context, subject, propio string) ([]string, error) {
	var out []string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT DISTINCT tenant FROM channel_members
			 WHERE subject = $1 AND externo AND tenant <> $2 ORDER BY tenant`, subject, propio)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

// enCadaAnfitrion recorre las organizaciones anfitrionas en UNA transacción,
// fijando la RLS de cada una antes de su consulta. fn devuelve true para parar.
func (s *Store) enCadaAnfitrion(ctx context.Context, subject, propio string, fn func(tx pgx.Tx, tenant string) (bool, error)) error {
	tenants, err := s.tenantsAnfitriones(ctx, subject, propio)
	if err != nil || len(tenants) == 0 {
		return err
	}
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		for _, t := range tenants {
			if err := pg.SetTenant(ctx, tx, t); err != nil {
				return err
			}
			parar, err := fn(tx, t)
			if err != nil || parar {
				return err
			}
		}
		return nil
	})
}

// AnfitrionDeCanal devuelve la organización donde vive el canal cuando subject está
// en él como persona de fuera; "" en cualquier otro caso, que es el normal.
func (s *Store) AnfitrionDeCanal(ctx context.Context, subject, propio string, channelID uuid.UUID) (string, error) {
	var tenant string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT tenant FROM channel_members
			 WHERE channel_id = $1 AND subject = $2 AND externo AND tenant <> $3`,
			channelID, subject, propio).Scan(&tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return tenant, err
}

// AnfitrionDeMensaje es lo mismo para un mensaje (o una respuesta de hilo). Un
// mensaje no dice de qué canal es hasta leerlo, y leerlo exige su organización:
// se busca en las pocas donde la persona es externa, cada una con su RLS.
func (s *Store) AnfitrionDeMensaje(ctx context.Context, subject, propio string, messageID uuid.UUID) (string, error) {
	var anfitrion string
	err := s.enCadaAnfitrion(ctx, subject, propio, func(tx pgx.Tx, t string) (bool, error) {
		var hay bool
		err := tx.QueryRow(ctx,
			`SELECT EXISTS (
			     SELECT 1 FROM messages m
			     JOIN channel_members cm
			       ON cm.tenant = m.tenant AND cm.channel_id = m.channel_id
			      AND cm.subject = $3 AND cm.externo
			     WHERE m.tenant = $1 AND m.id = $2)`, t, messageID, subject).Scan(&hay)
		if hay {
			anfitrion = t
		}
		return hay, err
	})
	return anfitrion, err
}

// AnfitrionDeSala es lo mismo para la sala de vídeo de una conversación: solo las
// salas NACIDAS de un canal donde la persona es externa. Una reunión suelta de la
// organización anfitriona no se abre a nadie de fuera por estar en uno de sus DM.
func (s *Store) AnfitrionDeSala(ctx context.Context, subject, propio string, roomID uuid.UUID) (string, error) {
	var anfitrion string
	err := s.enCadaAnfitrion(ctx, subject, propio, func(tx pgx.Tx, t string) (bool, error) {
		var hay bool
		err := tx.QueryRow(ctx,
			`SELECT EXISTS (
			     SELECT 1 FROM rooms r
			     JOIN channel_members cm
			       ON cm.tenant = r.tenant AND cm.subject = $3 AND cm.externo
			      AND cm.channel_id::text = r.metadata->'source'->>'ref_id'
			     WHERE r.tenant = $1 AND r.id = $2 AND r.metadata->'source'->>'kind' = 'chat')`,
			t, roomID, subject).Scan(&hay)
		if hay {
			anfitrion = t
		}
		return hay, err
	})
	return anfitrion, err
}

// AnfitrionDeReunionDeEvento es la organización donde vive la reunión de un evento de
// Calendar cuando no es la de quien pregunta: los invitados de otra organización entran
// en ella como invitados (decidido por el dueño el 2026-10-16). Solo se mira la
// organización; si entra lo decide la verja de la sala. "" si no es de un evento o es de
// la suya.
func (s *Store) AnfitrionDeReunionDeEvento(ctx context.Context, propio string, roomID uuid.UUID) (string, error) {
	var tenant string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT tenant FROM rooms
			 WHERE id = $1 AND tenant <> $2 AND metadata->'source'->>'kind' = 'calendar'`,
			roomID, propio).Scan(&tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return tenant, err
}

// DirectoDeFuera busca un DM con EXACTAMENTE ese conjunto de personas que ya viva en
// otra organización y donde subject esté como externo. Es lo que evita que Ana y
// Bea, de empresas distintas, acaben con dos conversaciones según quién escribió
// primero: el slug de un DM es el del conjunto y no depende de la organización.
func (s *Store) DirectoDeFuera(ctx context.Context, subject, propio string, subs []string) (domain.Channel, bool, error) {
	slug := directSlugN(subs...)
	var ch domain.Channel
	var hay bool
	err := s.enCadaAnfitrion(ctx, subject, propio, func(tx pgx.Tx, t string) (bool, error) {
		row := tx.QueryRow(ctx,
			`SELECT `+prefixed("c", channelCols)+` FROM channels c
			 WHERE c.tenant = $1 AND c.slug = $2 AND c.kind = 'direct' AND c.deleted_at IS NULL
			   AND EXISTS (SELECT 1 FROM channel_members cm
			               WHERE cm.tenant = c.tenant AND cm.channel_id = c.id
			                 AND cm.subject = $3 AND cm.externo)
			 LIMIT 1`, t, slug, subject)
		var err error
		ch, err = scanChannel(row, t)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		hay = err == nil
		ch.External = hay
		return hay, err
	})
	return ch, hay, err
}

// ListExternalChannels lista las conversaciones de OTRAS organizaciones donde
// subject está como persona de fuera, con sus no leídos: es lo que su barra
// lateral pinta junto a las suyas. Los nombres de un DM se resuelven por visor,
// igual que en ListChannels.
func (s *Store) ListExternalChannels(ctx context.Context, subject, propio string) ([]domain.Channel, []domain.ReadState, error) {
	var canales []domain.Channel
	var leidos []domain.ReadState
	err := s.enCadaAnfitrion(ctx, subject, propio, func(tx pgx.Tx, t string) (bool, error) {
		rows, err := tx.Query(ctx,
			`SELECT c.id, c.team_id, c.slug, c.name,
			        c.topic, c.purpose, c.kind, c.archived, c.created_by, c.created_at, c.updated_at, c.last_message_at,
			        mem.favorite, mem.notify_level,
			        COALESCE((
			             SELECT array_agg(COALESCE(NULLIF(cm2.name, ''), cm2.subject) ORDER BY cm2.name, cm2.subject)
			             FROM channel_members cm2
			             WHERE cm2.tenant=c.tenant AND cm2.channel_id=c.id AND cm2.subject <> $2
			               AND c.kind = 'direct'), '{}'::text[]),
			        COALESCE((
			             SELECT array_agg(cm3.subject ORDER BY cm3.name, cm3.subject)
			             FROM channel_members cm3
			             WHERE cm3.tenant=c.tenant AND cm3.channel_id=c.id AND cm3.subject <> $2
			               AND c.kind = 'direct'), '{}'::text[]),
			        COALESCE(rs.last_read_at, 'epoch'::timestamptz), rs.last_read_message_id,
			        (SELECT count(*) FROM messages m
			         WHERE m.tenant=c.tenant AND m.channel_id=c.id AND m.deleted_at IS NULL
			           AND m.author_sub <> $2
			           AND m.created_at > COALESCE(rs.last_read_at, 'epoch'::timestamptz)),
			        (SELECT count(*) FROM messages m
			         WHERE m.tenant=c.tenant AND m.channel_id=c.id AND m.deleted_at IS NULL
			           AND m.author_sub <> $2
			           AND (m.mentions @> ARRAY[$2]::text[] OR m.mentions_all)
			           AND m.created_at > COALESCE(rs.last_read_at, 'epoch'::timestamptz))
			 FROM channels c
			 JOIN channel_members mem
			   ON mem.tenant=c.tenant AND mem.channel_id=c.id AND mem.subject=$2 AND mem.externo
			 LEFT JOIN read_state rs
			   ON rs.tenant=c.tenant AND rs.channel_id=c.id AND rs.subject=$2
			 WHERE c.tenant=$1 AND c.deleted_at IS NULL AND c.archived = false
			 ORDER BY c.last_message_at DESC NULLS LAST, lower(c.name)`, t, subject)
		if err != nil {
			return false, err
		}
		defer rows.Close()
		for rows.Next() {
			var c domain.Channel
			var rs domain.ReadState
			var kind, notifyLevel string
			var peers, peerSubs []string
			if err := rows.Scan(&c.ID, &c.TeamID, &c.Slug, &c.Name, &c.Topic, &c.Purpose, &kind,
				&c.Archived, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.LastMessage,
				&c.Favorite, &notifyLevel, &peers, &peerSubs,
				&rs.LastReadAt, &rs.LastMessageID, &rs.Unread, &rs.Mentions); err != nil {
				return false, err
			}
			c.Kind = domain.ChannelKind(kind)
			c.NotifyLevel = notifyLevel
			c.Muted = notifyLevel == domain.NotifyLevelNone
			c.Tenant = t
			c.External = true
			if c.Kind == domain.ChannelDirect && len(peers) > 0 {
				c.Peers, c.PeerSubs = peers, peerSubs
				c.Name = strings.Join(peers, ", ")
			}
			rs.ChannelID, rs.Subject = c.ID, subject
			canales = append(canales, c)
			leidos = append(leidos, rs)
		}
		return false, rows.Err()
	})
	return canales, leidos, err
}

// SubjectsFueraDelEquipo devuelve, de los subs dados, los que no son miembros del
// team: los candidatos a ser personas de otra organización.
func (s *Store) SubjectsFueraDelEquipo(ctx context.Context, teamID uuid.UUID, subs []string) ([]string, error) {
	var out []string
	t := tenancy.MustFrom(ctx).Slug
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT s FROM unnest($2::text[]) AS s
			 WHERE NOT EXISTS (SELECT 1 FROM team_members m
			                   WHERE m.tenant = $1 AND m.team_id = $3 AND m.subject = s)`,
			t, subs, teamID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sub string
			if err := rows.Scan(&sub); err != nil {
				return err
			}
			out = append(out, sub)
		}
		return rows.Err()
	})
	return out, err
}
