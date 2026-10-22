// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ListMemberInvitations devuelve solo invitaciones vigentes. Las caducadas no
// se presentan como accionables y la purga periódica puede borrarlas después.
func (s *Store) ListMemberInvitations(ctx context.Context) ([]domain.MemberInvitation, error) {
	t := tenancy.MustFrom(ctx)
	out := make([]domain.MemberInvitation, 0, 8)
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, email, role, invited_by, expires_at, created_at, updated_at
			FROM member_invitations
			WHERE tenant=$1 AND expires_at > now()
			ORDER BY created_at DESC, id DESC`, t.Slug)
		if err != nil {
			return fmtErr("listar invitaciones", err)
		}
		defer rows.Close()
		for rows.Next() {
			var inv domain.MemberInvitation
			if err := rows.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.InvitedBy,
				&inv.ExpiresAt, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
				return fmtErr("leer invitación", err)
			}
			out = append(out, inv)
		}
		return rows.Err()
	})
	return out, err
}

// UpsertMemberInvitation refleja la última invitación emitida por Sytue ID.
// Reenviar rota el token en ID y actualiza esta fila sin crear duplicados.
func (s *Store) UpsertMemberInvitation(ctx context.Context, email string, role domain.Role, invitedBy string, expiresAt time.Time) (domain.MemberInvitation, error) {
	t := tenancy.MustFrom(ctx)
	email = strings.ToLower(strings.TrimSpace(email))
	var inv domain.MemberInvitation
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO member_invitations
			(tenant, email, role, invited_by, expires_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tenant, lower(email)) DO UPDATE SET
				role=EXCLUDED.role, invited_by=EXCLUDED.invited_by,
				expires_at=EXCLUDED.expires_at, updated_at=now()
			RETURNING id, email, role, invited_by, expires_at, created_at, updated_at`,
			t.Slug, email, role, invitedBy, expiresAt).
			Scan(&inv.ID, &inv.Email, &inv.Role, &inv.InvitedBy,
				&inv.ExpiresAt, &inv.CreatedAt, &inv.UpdatedAt)
	})
	return inv, err
}

// DeleteMemberInvitation revoca el espejo de Cloud por id y devuelve el correo
// para revocar también el token canónico en Sytue ID. RLS impide usar ids ajenos.
func (s *Store) DeleteMemberInvitation(ctx context.Context, id uuid.UUID) (domain.MemberInvitation, error) {
	t := tenancy.MustFrom(ctx)
	var inv domain.MemberInvitation
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `DELETE FROM member_invitations
			WHERE tenant=$1 AND id=$2
			RETURNING id, email, role, invited_by, expires_at, created_at, updated_at`,
			t.Slug, id).Scan(&inv.ID, &inv.Email, &inv.Role, &inv.InvitedBy,
			&inv.ExpiresAt, &inv.CreatedAt, &inv.UpdatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MemberInvitation{}, domain.ErrNotFound
	}
	return inv, err
}

// DeleteMemberInvitationByEmail limpia un espejo pendiente cuando Sytue ID
// confirma que el correo ya es miembro y Cloud puede materializarlo directamente.
func (s *Store) DeleteMemberInvitationByEmail(ctx context.Context, email string) error {
	t := tenancy.MustFrom(ctx)
	email = strings.ToLower(strings.TrimSpace(email))
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM member_invitations
			WHERE tenant=$1 AND lower(email)=$2`, t.Slug, email)
		return err
	})
}

// AcceptMemberInvitation materializa exactamente una invitación vigente para
// el email verificado del JWT y su `sub` estable. DELETE ... RETURNING hace la
// aceptación de un solo uso; la membresía se crea en la MISMA transacción, por
// lo que un fallo no consume la invitación. Devuelve accepted=false si no había
// coincidencia vigente.
func (s *Store) AcceptMemberInvitation(ctx context.Context, sub, email string) (domain.Membership, bool, error) {
	t := tenancy.MustFrom(ctx)
	email = strings.ToLower(strings.TrimSpace(email))
	var m domain.Membership
	accepted := false
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var role domain.Role
		err := tx.QueryRow(ctx, `DELETE FROM member_invitations
			WHERE tenant=$1 AND lower(email)=$2 AND expires_at > now()
			RETURNING role`, t.Slug, email).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmtErr("consumir invitación", err)
		}
		// El INSERT de abajo es un UPSERT: si el invitado YA es miembro le pisa el
		// rol. Aceptar una invitación de rol menor siendo el único owner degradaría
		// la organización a cero owners por una puerta que no pasa por AddMember, así
		// que la misma guarda cubre también esta. Rechazar aquí revierte la tx entera
		// y la invitación queda sin consumir.
		if err := guardLastOwner(ctx, tx, t.Slug, sub, role); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `INSERT INTO memberships (tenant, sub, role)
			VALUES ($1, $2, $3)
			ON CONFLICT (tenant, sub) DO UPDATE SET role=EXCLUDED.role
			RETURNING tenant, sub, role, created_at`, t.Slug, sub, role).
			Scan(&m.Tenant, &m.Sub, &m.Role, &m.CreatedAt)
		if err == nil {
			accepted = true
		}
		return err
	})
	return m, accepted, err
}
