// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// LA GENTE DE FUERA ACEPTA ANTES DE ENTRAR (decidido por el dueño el 2026-10-16: «Que
// acepte primero», también en mensajes directos y grupos; mig 0041).
//
// A una persona de OTRA organización ya no se la mete en channel_members: se le deja
// una invitación en la organización anfitriona, y hasta que la acepta no es miembro de
// nada. Ella pregunta desde SU organización, así que la lista se lee por la política de
// mantenimiento (solo lectura, siempre por el `sub` del token); aceptar y rechazar se
// hacen dentro de la anfitriona, con su RLS.

// invitar deja (o refresca) la invitación de `subject` al canal, dentro de la transacción
// de la anfitriona.
func invitar(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID, subject, name, actor, actorName string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO invitaciones_canal (tenant, channel_id, subject, name, invitado_por, nombre_de_quien_invita)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant, channel_id, subject) DO UPDATE SET name = EXCLUDED.name`,
		tenant, channelID, subject, name, actor, actorName)
	return err
}

// nombreEnElCanal es el nombre con el que se ve a `subject` en el canal ("" si no está).
func nombreEnElCanal(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID, subject string) string {
	var n string
	_ = tx.QueryRow(ctx, `SELECT name FROM channel_members WHERE tenant=$1 AND channel_id=$2 AND subject=$3`,
		tenant, channelID, subject).Scan(&n)
	return n
}

// invitacionesDe son las (organización, canal) a las que está invitada subject fuera de
// la suya. En régimen normal, ninguna.
func (s *Store) invitacionesDe(ctx context.Context, subject, propio string) ([]domain.Invitacion, error) {
	var out []domain.Invitacion
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT tenant, channel_id, invitado_por, nombre_de_quien_invita, created_at
			   FROM invitaciones_canal WHERE subject = $1 AND tenant <> $2
			  ORDER BY created_at DESC LIMIT 100`, subject, propio)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var i domain.Invitacion
			if err := rows.Scan(&i.Organizacion, &i.ChannelID, &i.InvitadoPor, &i.NombreDeQuienInvita, &i.Creada); err != nil {
				return err
			}
			out = append(out, i)
		}
		return rows.Err()
	})
	return out, err
}

// Invitaciones lista las invitaciones pendientes de subject, con el nombre y la clase
// del canal (lo único que se ve de él antes de aceptar).
func (s *Store) Invitaciones(ctx context.Context, subject, propio string) ([]domain.Invitacion, error) {
	lista, err := s.invitacionesDe(ctx, subject, propio)
	if err != nil || len(lista) == 0 {
		return lista, err
	}
	out := lista[:0]
	err = s.pool.InTx(ctx, func(tx pgx.Tx) error {
		for _, i := range lista {
			if err := pg.SetTenant(ctx, tx, i.Organizacion); err != nil {
				return err
			}
			ch, err := getChannelTx(ctx, tx, i.Organizacion, i.ChannelID)
			if errors.Is(err, domain.ErrNotFound) || (err == nil && ch.Archived) {
				continue
			}
			if err != nil {
				return err
			}
			i.Nombre, i.Clase = ch.Name, ch.Kind
			out = append(out, i)
		}
		return nil
	})
	return out, err
}

// InvitacionDelCanal es la invitación de subject al canal, leída en la organización
// anfitriona (la del contexto), con el nombre y la clase del canal: lo que lleva el aviso
// de la campana a quien invitan (COM-15). ErrNotFound si no hay.
func (s *Store) InvitacionDelCanal(ctx context.Context, channelID uuid.UUID, subject string) (domain.Invitacion, error) {
	t := tenancy.MustFrom(ctx)
	i := domain.Invitacion{Organizacion: t.Slug, ChannelID: channelID}
	var clase string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT c.name, c.kind, i.invitado_por, i.nombre_de_quien_invita, i.created_at
			   FROM invitaciones_canal i JOIN channels c ON c.tenant = i.tenant AND c.id = i.channel_id
			  WHERE i.tenant = $1 AND i.channel_id = $2 AND i.subject = $3`,
			t.Slug, channelID, subject).Scan(&i.Nombre, &clase, &i.InvitadoPor, &i.NombreDeQuienInvita, &i.Creada)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Invitacion{}, domain.ErrNotFound
	}
	i.Clase = domain.ChannelKind(clase)
	return i, err
}

// organizacionDeInvitacion es la anfitriona del canal al que está invitada subject; ""
// si no hay invitación.
func (s *Store) organizacionDeInvitacion(ctx context.Context, subject, propio string, channelID uuid.UUID) (string, error) {
	var tenant string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT tenant FROM invitaciones_canal WHERE channel_id = $1 AND subject = $2 AND tenant <> $3`,
			channelID, subject, propio).Scan(&tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return tenant, err
}

// AceptarInvitacion convierte la invitación en pertenencia (fila `externo`) y devuelve
// la organización anfitriona. ErrNotFound si no hay invitación.
func (s *Store) AceptarInvitacion(ctx context.Context, subject, propio string, channelID uuid.UUID) (string, error) {
	return s.resolverInvitacion(ctx, subject, propio, channelID, true)
}

// RechazarInvitacion la borra. ErrNotFound si no hay invitación.
func (s *Store) RechazarInvitacion(ctx context.Context, subject, propio string, channelID uuid.UUID) error {
	_, err := s.resolverInvitacion(ctx, subject, propio, channelID, false)
	return err
}

func (s *Store) resolverInvitacion(ctx context.Context, subject, propio string, channelID uuid.UUID, aceptar bool) (string, error) {
	tenant, err := s.organizacionDeInvitacion(ctx, subject, propio, channelID)
	if err != nil {
		return "", err
	}
	if tenant == "" {
		return "", domain.ErrNotFound
	}
	err = s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		var name string
		err := tx.QueryRow(ctx,
			`DELETE FROM invitaciones_canal WHERE tenant=$1 AND channel_id=$2 AND subject=$3 RETURNING name`,
			tenant, channelID, subject).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil || !aceptar {
			return err
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO channel_members (tenant, channel_id, subject, name, role, externo)
			 VALUES ($1, $2, $3, $4, 'member', true)
			 ON CONFLICT (tenant, channel_id, subject) DO NOTHING`,
			tenant, channelID, subject, name)
		return err
	})
	return tenant, err
}

// AceptarDirectoPendiente: quien abre un DM con un grupo que ya le había escrito desde
// otra organización acepta esa invitación en vez de abrir una segunda conversación (el
// mismo grupo, una sola conversación, ADR 0040). Abrirla es consentir.
func (s *Store) AceptarDirectoPendiente(ctx context.Context, subject, propio string, subs []string) (domain.Channel, bool, error) {
	lista, err := s.invitacionesDe(ctx, subject, propio)
	if err != nil || len(lista) == 0 {
		return domain.Channel{}, false, err
	}
	slug := directSlugN(subs...)
	for _, i := range lista {
		var ch domain.Channel
		err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(ctx, tx, i.Organizacion); err != nil {
				return err
			}
			var err error
			ch, err = getChannelTx(ctx, tx, i.Organizacion, i.ChannelID)
			return err
		})
		if errors.Is(err, domain.ErrNotFound) || (err == nil && (ch.Kind != domain.ChannelDirect || ch.Slug != slug)) {
			continue
		}
		if err != nil {
			return domain.Channel{}, false, err
		}
		if _, err := s.AceptarInvitacion(ctx, subject, propio, ch.ID); err != nil {
			return domain.Channel{}, false, err
		}
		ch.External = true
		return ch, true, nil
	}
	return domain.Channel{}, false, nil
}
