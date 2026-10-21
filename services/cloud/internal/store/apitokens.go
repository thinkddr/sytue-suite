// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// apiTokenCols son las columnas expuestas de un token de API. NUNCA se
// selecciona token_hash (no debe salir del proceso ni figurar en respuestas).
const apiTokenCols = `id, tenant, name, scope, created_by, expires_at, last_used_at, created_at, project_id` //nolint:gosec // G101: lista de columnas SQL, no una credencial.

func scanAPIToken(row pgx.Row, t string) (domain.APIToken, error) {
	var tok domain.APIToken
	if err := row.Scan(&tok.ID, &tok.Tenant, &tok.Name, &tok.Scope, &tok.CreatedBy,
		&tok.ExpiresAt, &tok.LastUsedAt, &tok.CreatedAt, &tok.ProjectID); err != nil {
		return domain.APIToken{}, err
	}
	tok.Tenant = t
	return tok, nil
}

// NewAPITokenRecord son los datos para persistir un token (solo el hash).
type NewAPITokenRecord struct {
	Name      string
	TokenHash string
	Scope     domain.TokenScope
	CreatedBy string
	ExpiresAt *time.Time
	// ProjectID (opcional) confina el token a un proyecto; nil = tenant-wide.
	ProjectID *uuid.UUID
}

// CreateAPIToken inserta un token de API del tenant activo y lo devuelve (sin el
// secreto en claro, que solo conoce el llamante).
func (s *Store) CreateAPIToken(ctx context.Context, rec NewAPITokenRecord) (domain.APIToken, error) {
	t := tenancy.MustFrom(ctx)
	var tok domain.APIToken
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO api_tokens (tenant, name, token_hash, scope, created_by, expires_at, project_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING `+apiTokenCols,
			t.Slug, rec.Name, rec.TokenHash, string(rec.Scope), rec.CreatedBy, rec.ExpiresAt, rec.ProjectID)
		var e error
		tok, e = scanAPIToken(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		return e
	})
	return tok, err
}

// ListAPITokens lista los tokens del tenant (metadatos; nunca el secreto).
func (s *Store) ListAPITokens(ctx context.Context) ([]domain.APIToken, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.APIToken
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+apiTokenCols+` FROM api_tokens WHERE tenant=$1 ORDER BY created_at DESC`, t.Slug)
		if err != nil {
			return fmtErr("listar tokens", err)
		}
		defer rows.Close()
		for rows.Next() {
			tok, err := scanAPIToken(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, tok)
		}
		return rows.Err()
	})
	return out, err
}

// DeleteAPIToken revoca (borra) un token del tenant. ErrNotFound si no existe.
func (s *Store) DeleteAPIToken(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM api_tokens WHERE tenant=$1 AND id=$2`, t.Slug, id)
		if err != nil {
			return fmtErr("revocar token", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ResolvedToken es el tenant/scope resueltos a partir del hash de un token de API
// (el portador no lleva tenant; se resuelve por el hash, como ResolveRepo).
type ResolvedToken struct {
	Tenant string
	Scope  domain.TokenScope
	ID     uuid.UUID
	// ProjectID != nil confina el token a un proyecto (enforcement en el middleware
	// de auth); nil = tenant-wide (sin cambio respecto al comportamiento previo).
	ProjectID *uuid.UUID
}

// ResolveAPIToken resuelve un token vivo (no expirado) por su hash SIN tenant en
// el contexto: usa una transacción de mantenimiento (pg.InMaintenanceTx) que
// habilita la política PERMISSIVE api_tokens_maintenance para leer cross-tenant
// (mismo mecanismo que el reconciliador de 0003; robusto con FORCE RLS y un dueño
// de tabla sin BYPASSRLS). Refresca last_used_at de forma acotada (a lo sumo una
// vez por minuto y token). Devuelve ErrNotFound si no hay token vivo con ese hash.
func (s *Store) ResolveAPIToken(ctx context.Context, tokenHash string) (ResolvedToken, error) {
	var rt ResolvedToken
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var scope string
		if err := tx.QueryRow(ctx, `
			SELECT tenant, scope, id, project_id FROM api_tokens
			WHERE token_hash=$1 AND (expires_at IS NULL OR expires_at > now())
			LIMIT 1`, tokenHash).Scan(&rt.Tenant, &scope, &rt.ID, &rt.ProjectID); err != nil {
			return err
		}
		rt.Scope = domain.TokenScope(scope)
		// Refresco best-effort de last_used_at (acotado; ignora error no fatal).
		_, _ = tx.Exec(ctx, `
			UPDATE api_tokens SET last_used_at=now()
			WHERE id=$1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, rt.ID)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ResolvedToken{}, domain.ErrNotFound
	}
	return rt, err
}
