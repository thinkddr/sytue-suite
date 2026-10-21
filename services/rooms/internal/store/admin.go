// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Administración de Rooms con alcance de ORGANIZACIÓN.
//
// Todo lo demás en este paquete se consulta desde la pertenencia: se ven los
// canales públicos y aquellos de los que uno es miembro (`ListChannels`:
// `c.kind = 'public' OR mem.subject IS NOT NULL`). Es lo correcto para el
// producto y deja al administrador de la organización sin nada: no puede ver un
// canal privado, ni restaurar uno archivado por error, ni saber cuántos hay.
//
// Estas dos consultas existen para esa superficie y solo para ella. Quién entra
// lo decide el middleware `RequireRole("owner","admin")` a la puerta; aquí NO se
// vuelve a comprobar la pertenencia, que es justo la diferencia.
//
// LOS MENSAJES DIRECTOS QUEDAN FUERA, y no por descuido: un DM es una
// conversación privada entre personas, no un espacio de trabajo de la empresa.
// Listarlos en una pantalla de administración enseñaría con quién habla cada
// cual —y cuándo— sin abrir un solo mensaje. Lo que un litigio necesita se pide
// por la vía de cumplimiento, con su retención legal y su traza de acceso, no
// desde una tabla de canales.
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

// AdminChannel es un canal visto desde la administración: el canal más lo que
// hace falta para decidir sobre él sin abrirlo —de qué equipo es, cuánta gente
// hay dentro y cuándo se habló por última vez—.
type AdminChannel struct {
	domain.Channel
	TeamName string `json:"team_name"`
	Members  int    `json:"members"`
}

// ListAllChannels devuelve TODOS los canales de la organización —públicos y
// privados, de cualquier equipo—, con los archivados detrás de una bandera
// porque son la excepción y ensucian la lista del día a día.
//
// El orden pone delante lo que se ha movido hace poco: quien entra aquí suele
// venir a por algo que acaba de pasar.
func (s *Store) ListAllChannels(ctx context.Context, includeArchived bool) ([]AdminChannel, error) {
	t := tenancy.MustFrom(ctx)
	var out []AdminChannel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT c.id, c.team_id, c.slug, c.name, c.topic, c.purpose, c.kind,
			        c.archived, c.created_by, c.created_at, c.updated_at, c.last_message_at,
			        COALESCE(t.name, ''),
			        (SELECT count(*) FROM channel_members m
			          WHERE m.tenant = c.tenant AND m.channel_id = c.id)
			   FROM channels c
			   LEFT JOIN teams t ON t.tenant = c.tenant AND t.id = c.team_id
			  WHERE c.tenant = $1 AND c.deleted_at IS NULL
			    AND c.kind <> 'direct'
			    AND ($2 OR c.archived = false)
			  ORDER BY c.archived, c.last_message_at DESC NULLS LAST, lower(c.name)`,
			t.Slug, includeArchived)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c AdminChannel
			var kind string
			if err := rows.Scan(&c.ID, &c.TeamID, &c.Slug, &c.Name, &c.Topic, &c.Purpose, &kind,
				&c.Archived, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.LastMessage,
				&c.TeamName, &c.Members); err != nil {
				return err
			}
			c.Kind = domain.ChannelKind(kind)
			c.Tenant = t.Slug
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetChannelArchivedAsAdmin archiva o restaura un canal SIN exigir pertenencia.
//
// `UpdateChannel` pasa por `requireChannelAdmin`, que es lo que hace falta desde
// el producto —un canal lo administra quien está dentro— y lo que deja al
// administrador de la organización sin poder deshacer un archivado en un canal
// privado del que no forma parte. Esta variante existe para ese caso y solo
// cambia `archived`: el nombre, el tema y el propósito los sigue llevando el
// equipo, que es quien sabe de qué va su canal.
func (s *Store) SetChannelArchivedAsAdmin(ctx context.Context, id uuid.UUID, archived bool) (domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	var ch domain.Channel
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		row := tx.QueryRow(ctx,
			`UPDATE channels SET archived = $3, updated_at = now()
			  WHERE tenant = $1 AND id = $2 AND deleted_at IS NULL
			    AND kind <> 'direct'
			 RETURNING `+channelCols,
			t.Slug, id, archived)
		var err error
		ch, err = scanChannel(row, t.Slug)
		if err != nil {
			return err
		}
		return s.revokeMatrixTx(ctx, tx, t.Slug, id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Cubre también el `kind <> 'direct'` del WHERE: pedir archivar un DM por
		// su id no es «no se puede», es que ese id no existe para esta pantalla.
		return domain.Channel{}, domain.ErrNotFound
	}
	return ch, err
}

// AdminStats es el resumen de arriba de la pantalla: cuántos canales hay y de
// qué tipo. Se calcula en la base y no contando el listado en Go, porque el
// listado esconde los archivados salvo que se pidan y el resumen debe decir
// cuántos hay aunque no se vean.
type AdminStats struct {
	Channels  int        `json:"channels"`
	Private   int        `json:"private"`
	Archived  int        `json:"archived"`
	Teams     int        `json:"teams"`
	LastEvent *time.Time `json:"last_event_at,omitempty"`
}

// Stats devuelve el resumen de Rooms para la organización.
func (s *Store) Stats(ctx context.Context) (AdminStats, error) {
	t := tenancy.MustFrom(ctx)
	var st AdminStats
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FILTER (WHERE kind <> 'direct'),
			        count(*) FILTER (WHERE kind = 'private'),
			        count(*) FILTER (WHERE kind <> 'direct' AND archived),
			        (SELECT count(*) FROM teams WHERE tenant = $1 AND deleted_at IS NULL),
			        max(last_message_at) FILTER (WHERE kind <> 'direct')
			   FROM channels
			  WHERE tenant = $1 AND deleted_at IS NULL`,
			t.Slug).Scan(&st.Channels, &st.Private, &st.Archived, &st.Teams, &st.LastEvent)
	})
	return st, err
}
