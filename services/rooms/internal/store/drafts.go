// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Borradores en servidor (paridad §4). Estado PER-USUARIO (subject) y cross-canal:
// un borrador por (canal, hilo). El cliente los sigue guardando en localStorage para
// restaurarlos al instante; esta tabla es la que los hace existir en el OTRO
// dispositivo y la que alimenta la vista «Borradores» del sidebar. Ver mig. 0026.

// nilParent es el sustituto de parent_id NULL en el índice único por expresión: en un
// UNIQUE normal dos NULL no colisionan, así que sin este COALESCE cada guardado del
// borrador de canal insertaría una fila nueva en vez de pisar la anterior.
const nilParent = `'00000000-0000-0000-0000-000000000000'::uuid`

// ListDrafts devuelve los borradores VIVOS del usuario (canal accesible y no
// borrado), del más reciente al más antiguo. Reaplica el filtro de acceso al canal
// como ListSaved: si te sacaron de un canal privado, su borrador deja de listarse.
func (s *Store) ListDrafts(ctx context.Context, subject string) ([]domain.Draft, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Draft
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT d.channel_id, d.parent_id, d.body, d.updated_at
			 FROM message_drafts d
			 JOIN channels c ON c.tenant=d.tenant AND c.id=d.channel_id
			 WHERE d.tenant=$1 AND d.subject=$2 AND c.deleted_at IS NULL
			   AND (
			        c.kind='public'
			        OR EXISTS (SELECT 1 FROM channel_members cm
			                   WHERE cm.tenant=c.tenant AND cm.channel_id=c.id AND cm.subject=$2)
			   )
			 ORDER BY d.updated_at DESC`,
			t.Slug, subject)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d domain.Draft
			if err := rows.Scan(&d.ChannelID, &d.ParentID, &d.Body, &d.UpdatedAt); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}

// PutDraft fija (upsert) el borrador del usuario en un ámbito de redacción. Exige
// acceso al canal —el mismo que para leerlo—: sin esa verja, message_drafts sería un
// almacén libre indexado por un canal ajeno. El cuerpo ya viene recortado y acotado
// por el servicio.
func (s *Store) PutDraft(ctx context.Context, subject string, channelID uuid.UUID, parentID *uuid.UUID, body string) (domain.Draft, error) {
	t := tenancy.MustFrom(ctx)
	d := domain.Draft{ChannelID: channelID, ParentID: parentID, Body: body}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`INSERT INTO message_drafts (tenant, subject, channel_id, parent_id, body, updated_at)
			 VALUES ($1, $2, $3, $4, $5, now())
			 ON CONFLICT (tenant, subject, channel_id, COALESCE(parent_id, `+nilParent+`))
			 DO UPDATE SET body=EXCLUDED.body, updated_at=now()
			 RETURNING updated_at`,
			t.Slug, subject, channelID, parentID, body).Scan(&d.UpdatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Draft{}, domain.ErrNotFound
	}
	return d, err
}

// DeleteDraft borra el borrador de un ámbito (al enviarlo o al vaciar el composer).
// Idempotente: borrar lo que no hay no es un error.
func (s *Store) DeleteDraft(ctx context.Context, subject string, channelID uuid.UUID, parentID *uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`DELETE FROM message_drafts
			  WHERE tenant=$1 AND subject=$2 AND channel_id=$3
			    AND COALESCE(parent_id, `+nilParent+`) = COALESCE($4::uuid, `+nilParent+`)`,
			t.Slug, subject, channelID, parentID)
		return err
	})
}
