// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

const invitationCols = `id, event_id, uid, method, organizer, recipient, recipient_sub,
	sequence_no, status, created_at, responded_at`

func scanInvitation(row pgx.Row, t string) (domain.Invitation, error) {
	var inv domain.Invitation
	var method, status string
	if err := row.Scan(&inv.ID, &inv.EventID, &inv.UID, &method, &inv.Organizer,
		&inv.Recipient, &inv.RecipientSub, &inv.SequenceNo, &status,
		&inv.CreatedAt, &inv.RespondedAt); err != nil {
		return domain.Invitation{}, err
	}
	inv.Tenant = t
	inv.Method = domain.SchedulingMethod(method)
	inv.Status = domain.InvitationStatus(status)
	return inv, nil
}

// NewInvitation son los datos de un mensaje de scheduling iTIP.
type NewInvitation struct {
	EventID      *uuid.UUID
	UID          string
	Method       domain.SchedulingMethod
	Organizer    string
	Recipient    string
	RecipientSub string
	SequenceNo   int
	Data         string
}

// CreateInvitation entrega una invitacion a la "scheduling inbox" del
// destinatario (RFC 6638). Es transaccional con el cambio del evento si se pasa
// la misma tx via CreateInvitationsTx.
func (s *Store) CreateInvitation(ctx context.Context, ni NewInvitation) (domain.Invitation, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidMethod(ni.Method) || strings.TrimSpace(ni.Recipient) == "" {
		return domain.Invitation{}, domain.ErrInvalidEvent
	}
	var inv domain.Invitation
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		inv, e = insertInvitation(ctx, tx, t.Slug, ni)
		return e
	})
	return inv, err
}

func insertInvitation(ctx context.Context, tx pgx.Tx, tenant string, ni NewInvitation) (domain.Invitation, error) {
	status := domain.InvDelivered
	if ni.Method == domain.MethodRequest {
		status = domain.InvPending
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO invitations (tenant, event_id, uid, method, organizer, recipient, recipient_sub, sequence_no, data, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING `+invitationCols,
		tenant, ni.EventID, ni.UID, string(ni.Method), ni.Organizer,
		strings.TrimSpace(ni.Recipient), ni.RecipientSub, ni.SequenceNo, ni.Data, string(status))
	return scanInvitation(row, tenant)
}

// ListInbox lista las invitaciones de un destinatario (scheduling inbox), mas
// recientes primero. Si onlyPending, filtra status='pending'.
func (s *Store) ListInbox(ctx context.Context, recipient string, onlyPending bool, limit int) ([]domain.Invitation, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []domain.Invitation
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+invitationCols+` FROM invitations
			 WHERE tenant=$1 AND lower(recipient)=lower($2)
			   AND ($3 = false OR status = 'pending')
			 ORDER BY created_at DESC LIMIT $4`,
			t.Slug, strings.TrimSpace(recipient), onlyPending, limit)
		if err != nil {
			return fmt.Errorf("store: listar inbox: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			inv, err := scanInvitation(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, inv)
		}
		return rows.Err()
	})
	return out, err
}

// GetInvitation devuelve una invitacion por ID.
func (s *Store) GetInvitation(ctx context.Context, id uuid.UUID) (domain.Invitation, error) {
	t := tenancy.MustFrom(ctx)
	var inv domain.Invitation
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+invitationCols+` FROM invitations WHERE tenant=$1 AND id=$2`, t.Slug, id)
		var e error
		inv, e = scanInvitation(row, t.Slug)
		return e
	})
	if isNoRows(err) {
		return domain.Invitation{}, domain.ErrNotFound
	}
	return inv, err
}

// RespondInvitation marca una invitacion como respondida (accepted/declined/
// tentative) y, si lleva evento asociado, actualiza el PARTSTAT del asistente.
// Solo el DESTINATARIO responde: la fila debe casar el correo (recipient) o el
// sub (recipient_sub) del JWT del llamante; una identidad vacia nunca casa
// (deny-by-default). Sin coincidencia → ErrNotFound, sin distinguir «no
// existe» de «no es tuya» (anti-enumeracion).
func (s *Store) RespondInvitation(ctx context.Context, id uuid.UUID, status domain.InvitationStatus, callerEmails []string, callerSub string) (domain.Invitation, error) {
	t := tenancy.MustFrom(ctx)
	switch status {
	case domain.InvAccepted, domain.InvDeclined, domain.InvTentative:
	default:
		return domain.Invitation{}, domain.ErrInvalidEvent
	}
	direcciones := make([]string, 0, len(callerEmails))
	for _, e := range callerEmails {
		if e = domain.NormalizeEmail(e); e != "" {
			direcciones = append(direcciones, e)
		}
	}
	var inv domain.Invitation
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE invitations SET status=$3, responded_at=now()
			 WHERE tenant=$1 AND id=$2 AND status='pending'
			   AND (lower(recipient) = ANY($4) OR (recipient_sub <> '' AND recipient_sub = $5))
			 RETURNING `+invitationCols,
			t.Slug, id, string(status), direcciones, callerSub)
		var e error
		inv, e = scanInvitation(row, t.Slug)
		if e != nil {
			return e
		}
		// Refleja la respuesta en el PARTSTAT del asistente del evento.
		if inv.EventID != nil {
			part := invStatusToPartStat(status)
			if _, e := tx.Exec(ctx,
				`UPDATE attendees SET partstat=$3, updated_at=now()
				 WHERE tenant=$1 AND event_id=$2 AND lower(email)=lower($4)`,
				t.Slug, *inv.EventID, string(part), inv.Recipient); e != nil {
				return fmt.Errorf("store: actualizar partstat del asistente: %w", e)
			}
		}
		return nil
	})
	if isNoRows(err) {
		return domain.Invitation{}, domain.ErrNotFound
	}
	return inv, err
}

func invStatusToPartStat(s domain.InvitationStatus) domain.ParticipationStatus {
	switch s {
	case domain.InvAccepted:
		return domain.PartAccepted
	case domain.InvDeclined:
		return domain.PartDeclined
	case domain.InvTentative:
		return domain.PartTentative
	default:
		return domain.PartNeedsAction
	}
}

// EsInvitadoDeEvento dice si alguna de las direcciones, o el sub, organiza el evento de
// ese UID o figura en él: como asistente o como destinatario de su invitación. Es lo que
// abre a sus invitados la reunión suelta del evento en Rooms (aunque el enlace que tienen
// sea de antes de que llevara clave).
func (s *Store) EsInvitadoDeEvento(ctx context.Context, uid, sub string, direcciones []string) (bool, error) {
	t := tenancy.MustFrom(ctx)
	var si bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM events e
			                 WHERE e.tenant=$1 AND e.uid=$2
			                   AND lower(regexp_replace(e.organizer, '^mailto:', '', 'i')) = ANY($3))
			     OR EXISTS (SELECT 1 FROM attendees a JOIN events e ON e.tenant=a.tenant AND e.id=a.event_id
			                 WHERE a.tenant=$1 AND e.tenant=$1 AND e.uid=$2
			                   AND lower(regexp_replace(a.email, '^mailto:', '', 'i')) = ANY($3))
			     OR EXISTS (SELECT 1 FROM invitations i
			                 WHERE i.tenant=$1 AND i.uid=$2
			                   AND (lower(i.recipient) = ANY($3) OR ($4 <> '' AND i.recipient_sub = $4)))`,
			t.Slug, uid, direcciones, sub).Scan(&si)
	})
	return si, err
}

// InvitacionPendiente es la invitación (REQUEST) aún sin contestar de ese evento a esa
// dirección: la que un REPLY por correo (iMIP) contesta. La más reciente si hay varias.
func (s *Store) InvitacionPendiente(ctx context.Context, uid, email string) (uuid.UUID, error) {
	t := tenancy.MustFrom(ctx)
	var id uuid.UUID
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id FROM invitations
			 WHERE tenant=$1 AND uid=$2 AND lower(recipient)=lower($3)
			   AND method='REQUEST' AND status='pending'
			 ORDER BY created_at DESC LIMIT 1`,
			t.Slug, uid, strings.TrimSpace(email)).Scan(&id)
	})
	if isNoRows(err) {
		return uuid.Nil, domain.ErrNotFound
	}
	return id, err
}
