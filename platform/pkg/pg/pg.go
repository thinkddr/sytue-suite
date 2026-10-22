// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package pg envuelve PostgreSQL (único datastore de Sytue One) sobre pgx:
// pool de conexiones, helpers de transacción, integración con tenancy/RLS y el
// runner de migraciones. Sin Redis/Valkey: estado, colas y caché viven aquí.
package pg

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DirectDSNEnv es la variable de entorno (la inyecta el despliegue, no la
// config por servicio) con el DSN DIRECTO a PostgreSQL cuando el DSN principal
// pasa por un pooler transaccional (PgBouncer en prod, pool_mode=transaction).
// El trabajo con estado de SESIÓN no sobrevive a ese modo: LISTEN/NOTIFY
// (AcquireDirect) y los advisory locks de las migraciones (NewMigrator).
// Vacía (dev/compose, sin PgBouncer) => se usa el DSN principal, que ya es
// una conexión directa.
const DirectDSNEnv = "DATABASE_DIRECT_URL"

// directDSN devuelve el DSN de bypass del pooler si está definido en el
// entorno; si no, el DSN dado.
func directDSN(dsn string) string {
	if d := os.Getenv(DirectDSNEnv); d != "" {
		return d
	}
	return dsn
}

// Config configura el pool de conexiones.
type Config struct {
	// DSN es la cadena de conexión PostgreSQL (postgres://user:pass@host/db).
	DSN string
	// MaxConns es el tamaño máximo del pool (por defecto 10; se acota a
	// [1, 4096] para tolerar valores absurdos venidos del entorno).
	MaxConns int
	// MinConns son conexiones mínimas mantenidas (por defecto 0).
	MinConns int32
	// MaxConnLifetime recicla conexiones tras este tiempo.
	MaxConnLifetime time.Duration
	// MaxConnIdleTime cierra conexiones ociosas tras este tiempo.
	MaxConnIdleTime time.Duration
	// HealthCheckPeriod controla el ping periódico del pool.
	HealthCheckPeriod time.Duration
}

// Querier es la superficie común de pgxpool.Pool, pgx.Conn y pgx.Tx, de modo
// que los stores se escriben una vez y funcionan dentro o fuera de transacción.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Pool envuelve *pgxpool.Pool con helpers de Sytue One. Si DATABASE_DIRECT_URL
// está definida (prod detrás de PgBouncer), mantiene además un pool pequeño
// DIRECTO a PostgreSQL para trabajo con estado de sesión (ver AcquireDirect).
type Pool struct {
	*pgxpool.Pool
	direct *pgxpool.Pool
}

// Connect abre el pool y verifica la conectividad con un ping.
func Connect(ctx context.Context, cfg Config) (*Pool, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("pg: DSN vacío")
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("pg: DSN inválido: %w", err)
	}

	switch {
	case cfg.MaxConns > 0 && cfg.MaxConns <= 4096:
		poolCfg.MaxConns = int32(cfg.MaxConns) //nolint:gosec // G115: acotado a [1,4096] por el case.
	case cfg.MaxConns > 4096:
		poolCfg.MaxConns = 4096
	default:
		poolCfg.MaxConns = 10
	}
	poolCfg.MinConns = cfg.MinConns
	if cfg.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	} else {
		poolCfg.MaxConnLifetime = time.Hour
	}
	if cfg.MaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	} else {
		poolCfg.MaxConnIdleTime = 30 * time.Minute
	}
	if cfg.HealthCheckPeriod > 0 {
		poolCfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("pg: no se pudo crear el pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pg: ping fallido: %w", err)
	}

	if err := verificarNoEludeRLS(pingCtx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	p := &Pool{Pool: pool}
	if direct := directDSN(cfg.DSN); direct != cfg.DSN {
		dcfg, err := pgxpool.ParseConfig(direct)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("pg: %s inválido: %w", DirectDSNEnv, err)
		}
		// ponytail: pool mínimo — LISTEN mantiene 1 conexión viva por réplica
		// más margen de reconexión. Las migraciones abren su propia conexión.
		dcfg.MaxConns = 3
		dcfg.MinConns = 0
		dcfg.MaxConnLifetime = poolCfg.MaxConnLifetime
		dcfg.MaxConnIdleTime = poolCfg.MaxConnIdleTime
		dpool, err := pgxpool.NewWithConfig(ctx, dcfg)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("pg: no se pudo crear el pool directo: %w", err)
		}
		if err := dpool.Ping(pingCtx); err != nil {
			dpool.Close()
			pool.Close()
			return nil, fmt.Errorf("pg: ping directo fallido: %w", err)
		}
		if err := verificarNoEludeRLS(pingCtx, dpool); err != nil {
			dpool.Close()
			pool.Close()
			return nil, err
		}
		p.direct = dpool
	}
	return p, nil
}

// AcquireDirect entrega una conexión del pool DIRECTO a PostgreSQL (bypass del
// pooler transaccional) para trabajo con estado de sesión: LISTEN/NOTIFY del
// bus de eventos, advisory locks, etc. Sin DATABASE_DIRECT_URL (dev/compose)
// degrada al pool principal, cuya conexión ya es directa.
func (p *Pool) AcquireDirect(ctx context.Context) (*pgxpool.Conn, error) {
	if p.direct != nil {
		return p.direct.Acquire(ctx)
	}
	return p.Acquire(ctx)
}

// Close cierra el pool principal y el pool directo (si existe).
func (p *Pool) Close() {
	if p.direct != nil {
		p.direct.Close()
	}
	p.Pool.Close()
}

// Healthz comprueba la conectividad (para sondas /readyz).
func (p *Pool) Healthz(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return p.Ping(ctx)
}
