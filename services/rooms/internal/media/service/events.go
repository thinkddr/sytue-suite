// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// PoolPublisher adapta el *pg.Pool al interfaz EventPublisher publicando cada
// evento en su propia transacción sobre el outbox (events.PublishTx). Sobre
// PostgreSQL, sin Redis/Valkey (CONVENTIONS §3).
type PoolPublisher struct {
	pool *pg.Pool
}

// NewPoolPublisher construye un PoolPublisher.
func NewPoolPublisher(pool *pg.Pool) *PoolPublisher { return &PoolPublisher{pool: pool} }

// Publish escribe el evento en el outbox de PostgreSQL.
func (p *PoolPublisher) Publish(ctx context.Context, ev events.Event) error {
	return events.PublishTx(ctx, p.pool, ev)
}
