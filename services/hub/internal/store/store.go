// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store implementa la persistencia de Sytue Hub sobre PostgreSQL (pgx):
// notificaciones unificadas y presencia. Todo acceso pasa por transacciones con
// `app.tenant` fijado (pg.InTenantTx) para que RLS aísle por tenant. Consultas
// SIEMPRE parametrizadas (sin SQL dinámico), conforme a CONVENTIONS §10.
//
// Sin Redis/Valkey: el estado efímero (presencia) y las colas/avisos viven en
// PostgreSQL; el fan-out en tiempo real se hace por LISTEN/NOTIFY.
package store

import (
	"context"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Store agrupa el acceso a datos de Hub.
type Store struct {
	pool *pg.Pool
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Pool expone el pool subyacente (lo usa el relay LISTEN/NOTIFY de notify).
func (s *Store) Pool() *pg.Pool { return s.pool }

// Healthz delega en el pool (para sondas /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }
