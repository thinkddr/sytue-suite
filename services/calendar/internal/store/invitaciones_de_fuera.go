// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// LAS INVITACIONES DE OTRA ORGANIZACIÓN (COM-15, decidido por el dueño el 2026-10-17; mig
// 0015). Una invitación vive en la organización que convoca. Quien la recibe desde otra
// la busca desde la suya por la política de mantenimiento, de solo lectura, y siempre
// por SUS direcciones personales y su `sub` —los del token verificado, nunca los de la
// petición—: no ve más que sus invitaciones. Responder se hace dentro de la anfitriona.

// InvitacionesDeFuera son las invitaciones (REQUEST) de organizaciones distintas de
// `propio` a alguna de `direcciones` (en minúsculas) o a `sub`; con `soloPendientes`,
// las que esperan respuesta. Cada una lleva su organización (Tenant) y el iCalendar de
// su REQUEST (Data), que es de donde sale lo que se ve de ella: su evento no es legible
// desde aquí.
func (s *Store) InvitacionesDeFuera(ctx context.Context, propio string, direcciones []string, sub string, soloPendientes bool, limite int) ([]domain.Invitation, error) {
	var out []domain.Invitation
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+invitationCols+`, tenant, data FROM invitations
			  WHERE tenant <> $1 AND method = 'REQUEST'
			    AND (lower(recipient) = ANY($2) OR ($3 <> '' AND recipient_sub = $3))
			    AND ($4 = false OR status = 'pending')
			  ORDER BY created_at DESC LIMIT $5`,
			propio, direcciones, sub, soloPendientes, limite)
		if err != nil {
			return fmt.Errorf("store: invitaciones de fuera: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var inv domain.Invitation
			var method, status string
			if err := rows.Scan(&inv.ID, &inv.EventID, &inv.UID, &method, &inv.Organizer,
				&inv.Recipient, &inv.RecipientSub, &inv.SequenceNo, &status,
				&inv.CreatedAt, &inv.RespondedAt, &inv.Tenant, &inv.Data); err != nil {
				return err
			}
			inv.Method = domain.SchedulingMethod(method)
			inv.Status = domain.InvitationStatus(status)
			out = append(out, inv)
		}
		return rows.Err()
	})
	return out, err
}

// OrganizacionDeInvitacion es la organización (distinta de `propio`) donde vive la
// invitación `id` a alguna de `direcciones` o a `sub`; "" si no hay ninguna así.
func (s *Store) OrganizacionDeInvitacion(ctx context.Context, id uuid.UUID, propio string, direcciones []string, sub string) (string, error) {
	var tenant string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT tenant FROM invitations
			  WHERE id = $1 AND tenant <> $2
			    AND (lower(recipient) = ANY($3) OR ($4 <> '' AND recipient_sub = $4))`,
			id, propio, direcciones, sub).Scan(&tenant)
		if isNoRows(err) {
			return nil
		}
		return err
	})
	return tenant, err
}
