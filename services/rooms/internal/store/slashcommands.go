// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

const slashCols = `id, name, description, usage_hint, request_url, token, signing_secret, created_by, created_at`

func (s *Store) scanSlash(row pgx.Row, tenant string) (domain.SlashCommand, error) {
	var c domain.SlashCommand
	if err := row.Scan(&c.ID, &c.Name, &c.Description, &c.UsageHint, &c.RequestURL,
		&c.Token, &c.SigningSecret, &c.CreatedBy, &c.CreatedAt); err != nil {
		return domain.SlashCommand{}, err
	}
	// El secreto de firma se guarda cifrado; aquí vuelve a texto para poder firmar.
	var err error
	if c.SigningSecret, err = s.openSecret(c.SigningSecret); err != nil {
		return domain.SlashCommand{}, err
	}
	c.Tenant = tenant
	return c, nil
}

// ErrSlashNameTaken indica que ese nombre ya existe en la organización. Se distingue de
// un error cualquiera porque es lo único que el usuario puede corregir por sí mismo.
var ErrSlashNameTaken = errors.New("store: ya hay un comando con ese nombre")

// CreateSlashCommand registra un comando. Solo un admin del workspace: un comando es
// tenant-wide y lo puede ejecutar cualquiera de la organización.
func (s *Store) CreateSlashCommand(ctx context.Context, c domain.SlashCommand, actor string) (domain.SlashCommand, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.SlashCommand
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if !esAdminDeLaOrganizacion(ctx, actor) {
			return domain.ErrForbidden
		}
		sealed, err := s.sealSecret(c.SigningSecret)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO slash_commands
			   (tenant, name, description, usage_hint, request_url, token, signing_secret, created_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+slashCols,
			t.Slug, c.Name, c.Description, c.UsageHint, c.RequestURL, c.Token, sealed, actor)
		out, err = s.scanSlash(row, t.Slug)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrSlashNameTaken
		}
		return err
	})
	return out, err
}

// ListSlashCommands devuelve los comandos de la organización, por nombre.
//
// No exige ser admin: el autocompletado del compositor los necesita para TODOS, y lo
// que se lista (nombre, descripción, uso) es justo lo que verá cualquiera al escribir
// «/». El secreto de firma lo recorta el servicio antes de responder.
func (s *Store) ListSlashCommands(ctx context.Context) ([]domain.SlashCommand, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.SlashCommand
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+slashCols+` FROM slash_commands WHERE tenant=$1 ORDER BY name`, t.Slug)
		if err != nil {
			return fmt.Errorf("store: listar comandos: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := s.scanSlash(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// IsWorkspaceAdmin expone el chequeo interno para que las capas de arriba decidan qué
// enseñar. Se usa, por ejemplo, para no mandarle a todo el mundo el secreto con el que
// se firman las ejecuciones de un comando.
func (s *Store) IsWorkspaceAdmin(ctx context.Context, actor string) (bool, error) {
	return esAdminDeLaOrganizacion(ctx, actor), nil
}

// SlashCommandByName resuelve el comando que hay que ejecutar.
func (s *Store) SlashCommandByName(ctx context.Context, name string) (domain.SlashCommand, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.SlashCommand
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		c, err = s.scanSlash(tx.QueryRow(ctx,
			`SELECT `+slashCols+` FROM slash_commands WHERE tenant=$1 AND name=$2`, t.Slug, name), t.Slug)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SlashCommand{}, domain.ErrNotFound
	}
	return c, err
}

// DeleteSlashCommand borra un comando (admin del workspace).
func (s *Store) DeleteSlashCommand(ctx context.Context, id uuid.UUID, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if !esAdminDeLaOrganizacion(ctx, actor) {
			return domain.ErrForbidden
		}
		tag, err := tx.Exec(ctx, `DELETE FROM slash_commands WHERE tenant=$1 AND id=$2`, t.Slug, id)
		if err != nil {
			return fmt.Errorf("store: borrar comando: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
