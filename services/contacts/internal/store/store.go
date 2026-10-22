// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store implementa la persistencia de Sytue Contacts sobre PostgreSQL
// (pgx). Todo acceso pasa por transacciones con `app.tenant` fijado
// (pg.InTenantTx) para que RLS aisle por tenant. Consultas SIEMPRE
// parametrizadas (sin SQL dinamico), conforme a CONVENTIONS §10.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Store agrupa el acceso a datos de Contacts.
type Store struct {
	pool *pg.Pool
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Healthz delega en el pool (para sondas /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// uniqueViolation indica si el error es una violacion de unicidad (23505).
func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// isNoRows es un atajo para pgx.ErrNoRows.
func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
