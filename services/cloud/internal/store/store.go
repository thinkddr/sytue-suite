// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store implementa la persistencia del control plane de Sytue Cloud
// sobre PostgreSQL (pgx). Todo acceso de cara al usuario pasa por transacciones
// con `app.tenant` fijado (pg.InTenantTx) para que RLS aísle por organización.
// Consultas SIEMPRE parametrizadas (sin SQL dinámico), conforme a CONVENTIONS §10.
//
// Los procesos de plano de control (cloud-builder, cloud-controller) que operan
// across-tenant fijan el tenant de la fila que reclaman mediante pg.SetTenant
// dentro de la misma transacción antes de mutar (ver claim de builds/deploys).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Store agrupa el acceso a datos de Sytue Cloud.
type Store struct {
	pool *pg.Pool
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Pool expone el pool subyacente (lo usan los relays/controller que necesitan
// transacciones cross-tenant o publicación de eventos).
func (s *Store) Pool() *pg.Pool { return s.pool }

// Healthz delega en el pool (para sondas /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// PageCursor es la clave de continuación de una paginación keyset por
// (created_at, id) DESC — listados de builds y deployments (CONVENTIONS §0.8:
// keyset estable, sin OFFSET). El cero-valor significa "primera página".
type PageCursor struct {
	// CreatedAt es el created_at de la última fila de la página anterior.
	CreatedAt time.Time
	// ID desempata filas con el mismo created_at de forma determinista.
	ID uuid.UUID
}

// IsZero indica que el cursor apunta al inicio (primera página).
func (c PageCursor) IsZero() bool { return c.ID == uuid.Nil && c.CreatedAt.IsZero() }

// uniqueViolation indica si el error es una violación de unicidad (23505).
func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// checkViolation indica si el error es una violación de CHECK (23514).
func checkViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}

// normalizeNoRows traduce pgx.ErrNoRows al error de dominio dado.
func normalizeNoRows(err, target error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return target
	}
	return err
}

// fmtErr envuelve un error de store con contexto.
func fmtErr(op string, err error) error {
	return fmt.Errorf("store: %s: %w", op, err)
}
