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

// Compartir calendarios con ACL. Un share concede a un destinatario (por su
// correo verificado y/o su sub) acceso reader|writer a un calendario del dueño.
// El enforcement (CalendarAccess) es la VERJA: dueño → writer; share que case
// sub/email → su permiso; si no → ErrNotFound (deny-by-default, sin filtrar
// existencia). Solo el dueño comparte/revoca (requireOwnedCalendar). Todo
// tenant-scoped por RLS (InTenantTx).

const shareCols = `id, calendar_id, grantee_email, grantee_sub, permission, created_by, created_at, updated_at`

func scanShare(row pgx.Row, t string) (domain.CalendarShare, error) {
	var sh domain.CalendarShare
	var perm string
	if err := row.Scan(&sh.ID, &sh.CalendarID, &sh.GranteeEmail, &sh.GranteeSub,
		&perm, &sh.CreatedBy, &sh.CreatedAt, &sh.UpdatedAt); err != nil {
		return domain.CalendarShare{}, err
	}
	sh.Tenant = t
	sh.Permission = domain.SharePermission(perm)
	return sh, nil
}

// requireOwnedCalendar verifica dentro de una tx que el calendario existe, está
// vivo y es del dueño dado. Si no lo es (o no existe), devuelve ErrNotFound: no
// distingue «no existe» de «no eres el dueño» para no filtrar la existencia de
// calendarios ajenos (deny-by-default). Es la puerta de la gestión de shares
// (crear/listar/revocar): solo el DUEÑO.
func requireOwnedCalendar(ctx context.Context, tx pgx.Tx, tenant string, calendarID uuid.UUID, ownerSub string) error {
	var owner string
	err := tx.QueryRow(ctx,
		`SELECT owner_sub FROM calendars WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
		tenant, calendarID).Scan(&owner)
	if isNoRows(err) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if ownerSub == "" || owner != ownerSub {
		return domain.ErrNotFound
	}
	return nil
}

// CreateCalendarShare comparte un calendario del dueño con un destinatario por
// correo (ya normalizado a minúsculas por el servicio) con el permiso dado. Solo
// el dueño (requireOwnedCalendar → ErrNotFound si no). Idempotente por
// destinatario: re-compartir con el mismo correo ACTUALIZA el permiso (upsert),
// así «cambiar de lector a editor» es un solo POST.
func (s *Store) CreateCalendarShare(ctx context.Context, calendarID uuid.UUID, ownerSub, granteeEmail string, perm domain.SharePermission) (domain.CalendarShare, error) {
	if !domain.ValidSharePermission(perm) {
		return domain.CalendarShare{}, domain.ErrInvalidShare
	}
	t := tenancy.MustFrom(ctx)
	var sh domain.CalendarShare
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireOwnedCalendar(ctx, tx, t.Slug, calendarID, ownerSub); err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO calendar_shares (tenant, calendar_id, grantee_email, permission, created_by)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (tenant, calendar_id, grantee_email, grantee_sub)
			 DO UPDATE SET permission = EXCLUDED.permission, updated_at = now()
			 RETURNING `+shareCols,
			t.Slug, calendarID, granteeEmail, string(perm), ownerSub)
		var e error
		sh, e = scanShare(row, t.Slug)
		return e
	})
	return sh, err
}

// ListCalendarShares lista los shares de un calendario del dueño (para «con quién
// está compartido» + revocar). Solo el dueño (ErrNotFound si no). Recientes
// primero, recorrido indexado por (tenant, calendar_id).
func (s *Store) ListCalendarShares(ctx context.Context, calendarID uuid.UUID, ownerSub string) ([]domain.CalendarShare, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.CalendarShare
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireOwnedCalendar(ctx, tx, t.Slug, calendarID, ownerSub); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT `+shareCols+` FROM calendar_shares
			 WHERE tenant=$1 AND calendar_id=$2
			 ORDER BY created_at DESC, id`,
			t.Slug, calendarID)
		if err != nil {
			return fmt.Errorf("store: listar shares del calendario: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			sh, err := scanShare(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, sh)
		}
		return rows.Err()
	})
	return out, err
}

// RevokeCalendarShare revoca un share de un calendario del dueño (corta el acceso
// del destinatario). Solo el dueño (ErrNotFound si no). ErrNotFound si el share no
// existe o no es de ese calendario.
func (s *Store) RevokeCalendarShare(ctx context.Context, calendarID, shareID uuid.UUID, ownerSub string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireOwnedCalendar(ctx, tx, t.Slug, calendarID, ownerSub); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM calendar_shares WHERE tenant=$1 AND id=$2 AND calendar_id=$3`,
			t.Slug, shareID, calendarID)
		if err != nil {
			return fmt.Errorf("store: revocar share: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// CalendarAccess es la VERJA de acceso a un calendario. Devuelve el permiso EFECTIVO
// del usuario (sub/email del JWT verificado) sobre el calendario:
//   - dueño → PermWriter (acceso total).
//   - share vigente que case su sub o su email → el permiso del share (el más
//     fuerte, writer>reader, si hubiera varios).
//   - ninguno → ErrNotFound (deny-by-default: 404, sin distinguir «no existe» de
//     «sin acceso», para no filtrar la existencia de calendarios ajenos).
//
// Autocontenido: no resuelve un directorio email→usuario; casa el correo/sub del
// share contra los claims del token. Un sub/email vacío nunca casa (las filas con
// esa identidad vacía se excluyen con `<> ”`). Tenant-scoped por RLS.
func (s *Store) CalendarAccess(ctx context.Context, sub, email string, calendarID uuid.UUID) (domain.SharePermission, error) {
	email = domain.NormalizeEmail(email)
	t := tenancy.MustFrom(ctx)
	var perm domain.SharePermission
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var owner string
		e := tx.QueryRow(ctx,
			`SELECT owner_sub FROM calendars WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, calendarID).Scan(&owner)
		if isNoRows(e) {
			return domain.ErrNotFound
		}
		if e != nil {
			return e
		}
		if sub != "" && owner == sub {
			perm = domain.PermWriter
			return nil
		}
		var p string
		e = tx.QueryRow(ctx,
			`SELECT permission FROM calendar_shares
			 WHERE tenant=$1 AND calendar_id=$2
			   AND ((grantee_sub <> '' AND grantee_sub = $3) OR (grantee_email <> '' AND grantee_email = $4))
			 ORDER BY (permission IN ('writer', 'manage')) DESC
			 LIMIT 1`,
			t.Slug, calendarID, sub, email).Scan(&p)
		if isNoRows(e) {
			return domain.ErrNotFound
		}
		if e != nil {
			return e
		}
		perm = domain.SharePermission(p)
		return nil
	})
	return perm, err
}

// SharedCalendar es un calendario compartido CON el usuario: el calendario del
// dueño más el permiso concedido y quién lo compartió. Lo consume «Compartido
// conmigo».
type SharedCalendar struct {
	Calendar   domain.Calendar        `json:"calendar"`
	Permission domain.SharePermission `json:"permission"`
	SharedBy   string                 `json:"shared_by"`
	// EditRequested: siendo lector, ya ha pedido editar (la franja lo dice).
	EditRequested bool `json:"edit_requested,omitempty"`
}

// ListCalendarsSharedWith lista los calendarios compartidos CON el usuario (por su
// sub o su correo del JWT), con su permiso y quién los compartió. Excluye los
// propios (por si un dueño se compartiera a sí mismo). Deduplicado por calendario
// con el permiso más fuerte (writer>reader) si hubiera varias vías. UNA consulta
// con JOIN (sin N+1); recorrido por los índices de destinatario.
func (s *Store) ListCalendarsSharedWith(ctx context.Context, sub, email string) ([]SharedCalendar, error) {
	email = domain.NormalizeEmail(email)
	t := tenancy.MustFrom(ctx)
	var out []SharedCalendar
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+calendarColsQual("c")+`, sh.permission, sh.created_by,
			        EXISTS (SELECT 1 FROM calendar_peticiones_edicion p WHERE p.tenant = c.tenant AND p.calendar_id = c.id AND p.sujeto = $2)
			   FROM calendar_shares sh
			   JOIN calendars c ON c.id = sh.calendar_id AND c.tenant = sh.tenant AND c.deleted_at IS NULL
			  WHERE sh.tenant = $1
			    AND ((sh.grantee_sub <> '' AND sh.grantee_sub = $2) OR (sh.grantee_email <> '' AND sh.grantee_email = $3))
			    AND c.owner_sub <> $2
			  ORDER BY (sh.permission IN ('writer', 'manage')) DESC, sh.created_at DESC`,
			t.Slug, sub, email)
		if err != nil {
			return fmt.Errorf("store: calendarios compartidos conmigo: %w", err)
		}
		defer rows.Close()
		seen := make(map[uuid.UUID]struct{})
		for rows.Next() {
			var c domain.Calendar
			var perm, sharedBy string
			var pedida bool
			if err := scanCalendarInto(rows, t.Slug, &c, &perm, &sharedBy, &pedida); err != nil {
				return err
			}
			if _, dup := seen[c.ID]; dup {
				continue // ya listado por una vía de permiso más fuerte (ORDER BY)
			}
			seen[c.ID] = struct{}{}
			out = append(out, SharedCalendar{
				Calendar:   c,
				Permission: domain.SharePermission(perm),
				SharedBy:   sharedBy,
				// Solo un lector tiene algo que pedir.
				EditRequested: pedida && perm == string(domain.PermReader),
			})
		}
		return rows.Err()
	})
	return out, err
}

// calendarColsQual devuelve calendarCols con cada columna cualificada por alias
// (para SELECT con JOIN). Debe listar las MISMAS columnas y en el MISMO orden que
// calendarCols/scanCalendar.
func calendarColsQual(alias string) string {
	cols := strings.Split(calendarCols, ",")
	for i, c := range cols {
		cols[i] = alias + "." + strings.TrimSpace(c)
	}
	return strings.Join(cols, ", ")
}

// scanCalendarInto escanea una fila «columnas de calendario + permiso + creado_por»
// (el SELECT de shared-with-me) en un domain.Calendar y sus dos extras. Replica el
// orden de scanCalendar y añade los dos campos del share.
func scanCalendarInto(row pgx.Row, t string, c *domain.Calendar, perm, sharedBy *string, pedida *bool) error {
	if err := row.Scan(&c.ID, &c.OwnerSub, &c.Name, &c.DisplayName, &c.Description,
		&c.Color, &c.Timezone, &c.Components, &c.IsDefault, &c.CTag, &c.SyncToken,
		&c.CreatedAt, &c.UpdatedAt, &c.ReadOnly, perm, sharedBy, pedida); err != nil {
		return err
	}
	c.Tenant = t
	return nil
}
