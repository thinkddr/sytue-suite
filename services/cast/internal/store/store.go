// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store es la persistencia de Sytue Cast sobre PostgreSQL (pgx). Cada
// función es UNA transacción que decide antes de tocar nada desde dónde lee:
//
//   - enTenant: la organización del vídeo (`app.tenant`), lectura y escritura.
//   - enPublico: `app.cast_publico='on'`, solo lectura de lo public/unlisted ya
//     visible, cruzando organizaciones (portada, canal, enlace sin sesión).
//   - enPersona: `app.cast_persona=<sub>`, solo lectura de los vídeos `people`
//     donde está esa persona, en la organización que sea.
//   - en mantenimiento (`pool.InMaintenanceTx`): la cola de transcodificación, que
//     cruza organizaciones a propósito.
//
// Consultas siempre parametrizadas y estáticas (CONVENTIONS §10). La RLS separa
// organizaciones; quién de DENTRO ve qué lo decide `internal/acceso`, y los
// listados repiten esa regla en su SQL.
package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// Store agrupa el acceso a datos de Sytue Cast.
type Store struct {
	pool *pg.Pool
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Healthz delega en el pool (para /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// DB expone el pool para lo que habla con Postgres sin pasar por el dominio: el
// limitador compartido de las rutas públicas y los paquetes hermanos (social),
// que abren sus propias transacciones con la misma regla.
func (s *Store) DB() *pg.Pool { return s.pool }

func (s *Store) enTenant(ctx context.Context, tenant string, fn func(tx pgx.Tx) error) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		return fn(tx)
	})
}

// enPublico es la ÚNICA función que fija `app.cast_publico`.
func (s *Store) enPublico(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.cast_publico', 'on', true)"); err != nil {
			return fmt.Errorf("cast: set_config app.cast_publico: %w", err)
		}
		return fn(tx)
	})
}

// enPersona es la ÚNICA función que fija `app.cast_persona`, con el sub del token.
func (s *Store) enPersona(ctx context.Context, sub string, fn func(tx pgx.Tx) error) error {
	if strings.TrimSpace(sub) == "" {
		return domain.ErrNoExiste
	}
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.cast_persona', $1, true)", sub); err != nil {
			return fmt.Errorf("cast: set_config app.cast_persona: %w", err)
		}
		return fn(tx)
	})
}

func esNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// esCheck: un 23514 es entrada inválida que se coló a la validación del dominio.
func esCheck(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}

// Cursor es la posición de paginación: (fecha, id) del último devuelto.
type Cursor struct {
	Fecha time.Time
	ID    uuid.UUID
}

// Cifrar lo deja opaco para el cable.
func (c Cursor) Cifrar() string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.Fecha.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

// DescifrarCursor acepta el vacío (desde el principio, nil).
func DescifrarCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // sin cursor = desde el principio.
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: cursor ilegible", domain.ErrEntradaInvalida)
	}
	fecha, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return nil, fmt.Errorf("%w: cursor incompleto", domain.ErrEntradaInvalida)
	}
	t, err := time.Parse(time.RFC3339Nano, fecha)
	if err != nil {
		return nil, fmt.Errorf("%w: fecha del cursor ilegible", domain.ErrEntradaInvalida)
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: id del cursor ilegible", domain.ErrEntradaInvalida)
	}
	return &Cursor{Fecha: t, ID: u}, nil
}

func argsCursor(c *Cursor) (*time.Time, uuid.UUID) {
	if c == nil {
		return nil, uuid.Nil
	}
	return &c.Fecha, c.ID
}
