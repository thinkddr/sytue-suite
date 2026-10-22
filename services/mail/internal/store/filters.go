// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// selectFilterCols son las columnas de una regla en orden de scanFilter.
const selectFilterCols = `id, tenant, account_id, name, enabled, priority,
	       match_from, match_subject, move_to, mark_read, mark_flagged`

func scanFilter(s scanner) (domain.Filter, error) {
	var f domain.Filter
	err := s.Scan(&f.ID, &f.Tenant, &f.AccountID, &f.Name, &f.Enabled, &f.Priority,
		&f.MatchFrom, &f.MatchSubject, &f.MoveTo, &f.MarkRead, &f.MarkFlagged)
	return f, err
}

// ListFilters devuelve TODAS las reglas de una cuenta (activas o no), en orden de
// evaluación, para la pantalla de gestión.
func (s *Store) ListFilters(ctx context.Context, accountID uuid.UUID) ([]domain.Filter, error) {
	return s.queryFilters(ctx, accountID, false)
}

// ListActiveFilters devuelve solo las reglas activas de una cuenta, en orden de
// prioridad ascendente. Es el camino caliente de la entrega entrante.
func (s *Store) ListActiveFilters(ctx context.Context, accountID uuid.UUID) ([]domain.Filter, error) {
	return s.queryFilters(ctx, accountID, true)
}

func (s *Store) queryFilters(ctx context.Context, accountID uuid.UUID, onlyEnabled bool) ([]domain.Filter, error) {
	// Defensa en profundidad: filtrar por tenant en la consulta ADEMÁS de RLS
	// (mismo criterio que AccountBySubject). Aísla incluso bajo un rol con BYPASSRLS
	// (superuser o mantenimiento), donde la política RLS no aplica.
	q := `SELECT ` + selectFilterCols + ` FROM mail_filters WHERE account_id = $1 AND tenant = $2`
	if onlyEnabled {
		q += ` AND enabled`
	}
	q += ` ORDER BY priority ASC, created_at ASC`
	var out []domain.Filter
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, accountID, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: list filters: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			f, err := scanFilter(rows)
			if err != nil {
				return err
			}
			out = append(out, f)
		}
		return rows.Err()
	})
	return out, err
}

// CreateFilter inserta una regla nueva. Valida el modelo de dominio y, si trae
// buzón destino, que pertenezca a la cuenta (defensa: la FK+RLS solo garantizan
// mismo tenant, no misma cuenta). Devuelve la regla persistida.
func (s *Store) CreateFilter(ctx context.Context, accountID uuid.UUID, f domain.Filter) (domain.Filter, error) {
	f.AccountID = accountID
	if err := f.Validate(); err != nil {
		return domain.Filter{}, err
	}
	var out domain.Filter
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := checkMoveTo(ctx, tx, accountID, f.MoveTo); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO mail_filters (
				tenant, account_id, name, enabled, priority,
				match_from, match_subject, move_to, mark_read, mark_flagged)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`+
			` RETURNING id, tenant, account_id, name, enabled, priority,
			           match_from, match_subject, move_to, mark_read, mark_flagged`,
			tenantOf(ctx), accountID, f.Name, f.Enabled, f.Priority,
			f.MatchFrom, f.MatchSubject, f.MoveTo, f.MarkRead, f.MarkFlagged)
		var err error
		out, err = scanFilter(row)
		return err
	})
	return out, err
}

// UpdateFilter reemplaza los campos mutables de una regla de una cuenta. Valida
// igual que CreateFilter. Devuelve ErrNotFound si la regla no existe/no es de la
// cuenta.
func (s *Store) UpdateFilter(ctx context.Context, accountID, id uuid.UUID, f domain.Filter) error {
	f.AccountID = accountID
	if err := f.Validate(); err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := checkMoveTo(ctx, tx, accountID, f.MoveTo); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE mail_filters SET
				name = $3, enabled = $4, priority = $5,
				match_from = $6, match_subject = $7,
				move_to = $8, mark_read = $9, mark_flagged = $10, updated_at = now()
			WHERE id = $1 AND account_id = $2 AND tenant = $11`,
			id, accountID, f.Name, f.Enabled, f.Priority,
			f.MatchFrom, f.MatchSubject, f.MoveTo, f.MarkRead, f.MarkFlagged, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: update filter: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// DeleteFilter borra una regla de una cuenta. Devuelve ErrNotFound si no existe.
func (s *Store) DeleteFilter(ctx context.Context, accountID, id uuid.UUID) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM mail_filters WHERE id = $1 AND account_id = $2 AND tenant = $3`, id, accountID, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: delete filter: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// checkMoveTo verifica que el buzón destino (si lo hay) existe y es de la cuenta.
func checkMoveTo(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, moveTo *uuid.UUID) error {
	if moveTo == nil {
		return nil
	}
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM mailboxes WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $2)`,
		*moveTo, accountID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("store: verificar buzón destino: %w", err)
	}
	if !exists {
		return domain.ErrInvalidFilter
	}
	return nil
}
