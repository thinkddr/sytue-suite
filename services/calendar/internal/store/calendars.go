// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

const calendarCols = `id, owner_sub, name, display_name, description, color, timezone,
	components, is_default, ctag, sync_token, created_at, updated_at, read_only`

func scanCalendar(row pgx.Row, t string) (domain.Calendar, error) {
	var c domain.Calendar
	if err := row.Scan(&c.ID, &c.OwnerSub, &c.Name, &c.DisplayName, &c.Description,
		&c.Color, &c.Timezone, &c.Components, &c.IsDefault, &c.CTag, &c.SyncToken,
		&c.CreatedAt, &c.UpdatedAt, &c.ReadOnly); err != nil {
		return domain.Calendar{}, err
	}
	c.Tenant = t
	return c, nil
}

// NewCalendar son los datos de creacion de un calendario.
type NewCalendar struct {
	OwnerSub    string
	Name        string
	DisplayName string
	Description string
	Color       string
	Timezone    string
	Components  []string
	IsDefault   bool
}

// CreateCalendar inserta un calendario para un dueno.
func (s *Store) CreateCalendar(ctx context.Context, nc NewCalendar) (domain.Calendar, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidName(nc.Name) {
		return domain.Calendar{}, domain.ErrInvalidName
	}
	if nc.Color == "" {
		nc.Color = "#6B3E9E"
	}
	if nc.Timezone == "" {
		nc.Timezone = "UTC"
	}
	if len(nc.Components) == 0 {
		nc.Components = []string{string(domain.CompEvent)}
	}
	var c domain.Calendar
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`INSERT INTO calendars (tenant, owner_sub, name, display_name, description, color, timezone, components, is_default)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 RETURNING `+calendarCols,
			t.Slug, nc.OwnerSub, strings.TrimSpace(nc.Name), nc.DisplayName, nc.Description,
			nc.Color, nc.Timezone, nc.Components, nc.IsDefault)
		var e error
		c, e = scanCalendar(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	return c, err
}

// EnsureDefaultCalendar devuelve el calendario por defecto del dueno, creando
// uno ("personal") si aun no tiene ninguno. Idempotente.
func (s *Store) EnsureDefaultCalendar(ctx context.Context, ownerSub string) (domain.Calendar, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.Calendar
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+calendarCols+` FROM calendars
			 WHERE tenant=$1 AND owner_sub=$2 AND is_default AND deleted_at IS NULL`,
			t.Slug, ownerSub)
		got, err := scanCalendar(row, t.Slug)
		if err == nil {
			c = got
			return nil
		}
		if !isNoRows(err) {
			return fmt.Errorf("store: leer calendario por defecto: %w", err)
		}
		insert := tx.QueryRow(ctx,
			`INSERT INTO calendars (tenant, owner_sub, name, display_name, is_default)
			 VALUES ($1, $2, 'personal', 'Personal', true)
			 RETURNING `+calendarCols,
			t.Slug, ownerSub)
		c, err = scanCalendar(insert, t.Slug)
		if err != nil {
			return fmt.Errorf("store: crear calendario por defecto: %w", err)
		}
		return nil
	})
	return c, err
}

// ListCalendars lista los calendarios vivos de un dueno.
func (s *Store) ListCalendars(ctx context.Context, ownerSub string) ([]domain.Calendar, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Calendar
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+calendarCols+` FROM calendars
			 WHERE tenant=$1 AND owner_sub=$2 AND deleted_at IS NULL
			 ORDER BY is_default DESC, lower(display_name), lower(name)`,
			t.Slug, ownerSub)
		if err != nil {
			return fmt.Errorf("store: listar calendarios: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanCalendar(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return attachSubscriptions(ctx, tx, t.Slug, out)
	})
	return out, err
}

// GetCalendar devuelve un calendario vivo por ID.
func (s *Store) GetCalendar(ctx context.Context, id uuid.UUID) (domain.Calendar, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.Calendar
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+calendarCols+` FROM calendars WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id)
		got, err := scanCalendar(row, t.Slug)
		if err != nil {
			return err
		}
		one := []domain.Calendar{got}
		if err := attachSubscriptions(ctx, tx, t.Slug, one); err != nil {
			return err
		}
		c = one[0]
		return nil
	})
	if isNoRows(err) {
		return domain.Calendar{}, domain.ErrNotFound
	}
	return c, err
}

// GetCalendarByName resuelve un calendario por (dueno, name) — ruta CalDAV.
func (s *Store) GetCalendarByName(ctx context.Context, ownerSub, name string) (domain.Calendar, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.Calendar
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+calendarCols+` FROM calendars
			 WHERE tenant=$1 AND owner_sub=$2 AND lower(name)=lower($3) AND deleted_at IS NULL`,
			t.Slug, ownerSub, name)
		var e error
		c, e = scanCalendar(row, t.Slug)
		return e
	})
	if isNoRows(err) {
		return domain.Calendar{}, domain.ErrNotFound
	}
	return c, err
}

// CalendarPatch son los campos editables de un calendario (nil = no cambia).
type CalendarPatch struct {
	DisplayName *string
	Description *string
	Color       *string
	Timezone    *string
}

// UpdateCalendar aplica un patch parcial a un calendario.
func (s *Store) UpdateCalendar(ctx context.Context, id uuid.UUID, p CalendarPatch) (domain.Calendar, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.Calendar
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE calendars SET
				display_name = COALESCE($3, display_name),
				description  = COALESCE($4, description),
				color        = COALESCE($5, color),
				timezone     = COALESCE($6, timezone),
				updated_at   = now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL
			 RETURNING `+calendarCols,
			t.Slug, id, p.DisplayName, p.Description, p.Color, p.Timezone)
		var e error
		c, e = scanCalendar(row, t.Slug)
		return e
	})
	if isNoRows(err) {
		return domain.Calendar{}, domain.ErrNotFound
	}
	return c, err
}

// DeleteCalendar marca un calendario como borrado (soft). El por defecto no se
// puede borrar.
func (s *Store) DeleteCalendar(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var isDefault bool
		if err := tx.QueryRow(ctx,
			`SELECT is_default FROM calendars WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id).Scan(&isDefault); err != nil {
			if isNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if isDefault {
			return domain.ErrDefaultImmutable
		}
		tag, err := tx.Exec(ctx,
			`UPDATE calendars SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, t.Slug, id)
		if err != nil {
			return fmt.Errorf("store: borrar calendario: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// bumpCalendar avanza ctag y sync_token de un calendario tras cualquier cambio
// de su contenido (debe llamarse dentro de la misma transaccion) para que
// sync-collection (RFC 6578) etiquete los cambios.
func bumpCalendar(ctx context.Context, tx pgx.Tx, tenant string, calendarID uuid.UUID) error {
	var token int64
	err := tx.QueryRow(ctx,
		`UPDATE calendars
		 SET ctag = gen_random_uuid(), sync_token = sync_token + 1, updated_at = now()
		 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL
		 RETURNING sync_token`, tenant, calendarID).Scan(&token)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: avanzar ctag/sync_token: %w", err)
	}
	return nil
}
