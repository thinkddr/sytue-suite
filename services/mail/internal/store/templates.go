// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Plantillas de respuesta (tabla mail_templates, migración 0021). CRUD
// owner-scoped por account_id además de RLS por tenant, como mail_labels. La
// validación de nombre y cuerpo es de domain.NormalizarTemplate; los CHECK de la
// tabla son la red de abajo.

func scanTemplate(s scanner) (domain.Template, error) {
	var tpl domain.Template
	err := s.Scan(&tpl.ID, &tpl.Tenant, &tpl.AccountID, &tpl.Name, &tpl.Body, &tpl.CreatedAt, &tpl.UpdatedAt)
	return tpl, err
}

// ListTemplates devuelve las plantillas de la cuenta por nombre, que es como las
// busca quien abre el menú del redactor.
func (s *Store) ListTemplates(ctx context.Context, accountID uuid.UUID) ([]domain.Template, error) {
	var out []domain.Template
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, account_id, name, body, created_at, updated_at
			FROM mail_templates
			WHERE account_id = $1 AND tenant = $2
			ORDER BY lower(name), id`, accountID, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: list templates: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			tpl, err := scanTemplate(rows)
			if err != nil {
				return err
			}
			out = append(out, tpl)
		}
		return rows.Err()
	})
	return out, err
}

// CreateTemplate crea una plantilla de la cuenta. Nombre repetido ⇒
// ErrTemplateExists; con TemplatesPorCuenta ya hechas ⇒ ErrTemplateLimite.
func (s *Store) CreateTemplate(ctx context.Context, accountID uuid.UUID, name, body string) (domain.Template, error) {
	name, body, err := domain.NormalizarTemplate(name, body)
	if err != nil {
		return domain.Template{}, err
	}
	var out domain.Template
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Se bloquea la fila de la CUENTA para contar: sin el bloqueo, dos altas a la
		// vez verían las dos 99 y el tope dejaría de ser un tope. De paso confirma que
		// la cuenta es de esta organización antes de escribir nada.
		var bloqueada uuid.UUID
		if err := tx.QueryRow(ctx, `
			SELECT id FROM accounts WHERE id = $1 AND tenant = $2 FOR UPDATE`,
			accountID, tenantOf(ctx)).Scan(&bloqueada); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("store: create template: %w", err)
		}
		var n int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM mail_templates WHERE account_id = $1 AND tenant = $2`,
			accountID, tenantOf(ctx)).Scan(&n); err != nil {
			return fmt.Errorf("store: create template: %w", err)
		}
		if n >= domain.TemplatesPorCuenta {
			return domain.ErrTemplateLimite
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO mail_templates (tenant, account_id, name, body)
			VALUES ($1, $2, $3, $4)
			RETURNING id, tenant, account_id, name, body, created_at, updated_at`,
			tenantOf(ctx), accountID, name, body)
		creada, err := scanTemplate(row)
		out = creada
		return err
	})
	if isUniqueViolation(err) {
		return domain.Template{}, domain.ErrTemplateExists
	}
	return out, err
}

// UpdateTemplate cambia el nombre y el cuerpo de una plantilla de la cuenta.
// ErrNotFound si no es suya; ErrTemplateExists si el nombre nuevo choca con otra.
func (s *Store) UpdateTemplate(ctx context.Context, accountID, id uuid.UUID, name, body string) (domain.Template, error) {
	name, body, err := domain.NormalizarTemplate(name, body)
	if err != nil {
		return domain.Template{}, err
	}
	var out domain.Template
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE mail_templates SET name = $3, body = $4, updated_at = now()
			WHERE id = $1 AND account_id = $2 AND tenant = $5
			RETURNING id, tenant, account_id, name, body, created_at, updated_at`,
			id, accountID, name, body, tenantOf(ctx))
		cambiada, err := scanTemplate(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		out = cambiada
		return err
	})
	if isUniqueViolation(err) {
		return domain.Template{}, domain.ErrTemplateExists
	}
	return out, err
}

// DeleteTemplate borra una plantilla de la cuenta. ErrNotFound si no es suya.
func (s *Store) DeleteTemplate(ctx context.Context, accountID, id uuid.UUID) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM mail_templates WHERE id = $1 AND account_id = $2 AND tenant = $3`,
			id, accountID, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: delete template: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
