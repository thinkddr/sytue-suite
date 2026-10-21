// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// RateLimiter es un limitador token-bucket compartido entre réplicas, respaldado
// por PostgreSQL (sin Redis/Valkey; el estado vive en el datastore único). La
// recarga y el consumo del token se resuelven de forma atómica en una única
// sentencia UPSERT, de modo que múltiples instancias del servicio comparten el
// mismo presupuesto por clave.
type RateLimiter struct {
	q     Querier
	rate  float64 // tokens por segundo
	burst float64
}

// RateLimiterConfig configura el limitador sobre Postgres.
type RateLimiterConfig struct {
	// PerSecond es la tasa sostenida de recarga (tokens/segundo).
	PerSecond float64
	// Burst es la capacidad máxima del bucket.
	Burst int
}

// NewRateLimiter crea un limitador sobre la tabla platform_rate_limits (creada
// por la migración de este paquete). Usa el Pool u otra Querier transaccional.
func NewRateLimiter(q Querier, cfg RateLimiterConfig) *RateLimiter {
	if cfg.PerSecond <= 0 {
		cfg.PerSecond = 1
	}
	if cfg.Burst <= 0 {
		cfg.Burst = 1
	}
	return &RateLimiter{q: q, rate: cfg.PerSecond, burst: float64(cfg.Burst)}
}

// rateLimitSQL recarga el bucket según el tiempo transcurrido, consume un token
// si hay disponible y devuelve los tokens restantes. Todo en una sentencia
// atómica (INSERT ... ON CONFLICT DO UPDATE ... RETURNING) con parámetros
// (sin SQL dinámico). Los parámetros: $1=key, $2=rate, $3=burst. La hora es la de la
// BASE (clock_timestamp), no la del proceso: con réplicas de
// relojes distintos, uno adelantado dejaría el cubo sin recargar hasta alcanzarlo.
const rateLimitSQL = `
INSERT INTO platform_rate_limits (bucket_key, tokens, updated_at)
VALUES ($1, $3 - 1, clock_timestamp())
ON CONFLICT (bucket_key) DO UPDATE SET
    tokens = LEAST(
        $3::double precision,
        -- GREATEST acota la deuda a -burst para que un cliente no acumule una
        -- penalización ilimitada tras una ráfaga sostenida (recuperación acotada).
        GREATEST(
            -$3::double precision,
            platform_rate_limits.tokens +
                -- GREATEST(0, …): una fila escrita con una hora posterior (otra
                -- réplica, antes de la hora de la base) no resta fichas.
                GREATEST(0, EXTRACT(EPOCH FROM (EXCLUDED.updated_at - platform_rate_limits.updated_at))) * $2::double precision
        )
    ) - 1,
    updated_at = GREATEST(platform_rate_limits.updated_at, EXCLUDED.updated_at)
RETURNING tokens`

// Allow implementa httpx.RateLimiter consumiendo un token de forma atómica en
// Postgres. Si tras la recarga los tokens quedan < 0, la petición se rechaza y
// se calcula el Retry-After (la fila deja constancia de la deuda, que se
// recupera con el tiempo).
func (rl *RateLimiter) Allow(ctx context.Context, key string) (httpx.RateLimitResult, error) {
	var tokens float64
	err := rl.q.QueryRow(ctx, rateLimitSQL, key, rl.rate, rl.burst).Scan(&tokens)
	if err != nil {
		return httpx.RateLimitResult{}, fmt.Errorf("pg: rate limit: %w", err)
	}
	res := httpx.RateLimitResult{Limit: int(rl.burst)}
	if tokens >= 0 {
		res.Allowed = true
		res.Remaining = int(tokens)
		return res, nil
	}
	// Deuda: tiempo hasta recuperar un token positivo.
	deficit := -tokens
	res.RetryAfter = time.Duration(deficit/rl.rate*float64(time.Second)) + time.Millisecond
	res.Remaining = 0
	return res, nil
}

// Las dos sentencias de AllowN: sembrar el cubo lleno si no existe y, con la fila ya
// bloqueada, recargarlo y gastar n SOLO si hay n. Así un cobro que no cabe no se deja
// fichas por el camino, y una petición de n cuesta dos viajes, no n.
const (
	rateLimitSembrarSQL = `
INSERT INTO platform_rate_limits (bucket_key, tokens, updated_at)
VALUES ($1, $2, clock_timestamp()) ON CONFLICT (bucket_key) DO NOTHING`
	rateLimitNSQL = `
WITH prev AS (
    SELECT tokens, updated_at, clock_timestamp() AS ahora
    FROM platform_rate_limits WHERE bucket_key = $1 FOR UPDATE
), calc AS (
    SELECT ahora, LEAST($3::double precision, GREATEST(-$3::double precision,
           tokens + GREATEST(0, EXTRACT(EPOCH FROM (ahora - updated_at))) * $2::double precision)) AS disp
    FROM prev
)
UPDATE platform_rate_limits t
SET tokens = CASE WHEN c.disp >= $4 THEN c.disp - $4 ELSE c.disp END,
    updated_at = GREATEST(t.updated_at, c.ahora)
FROM calc c
WHERE t.bucket_key = $1
RETURNING c.disp >= $4, t.tokens`
)

// AllowN cobra n fichas de golpe, y solo si las hay todas: si no, no gasta ninguna y
// dice cuánto esperar. Para lo que se cobra por elemento (el directorio interno de
// Sytue ID cobra por pareja dirección-candidato).
func (rl *RateLimiter) AllowN(ctx context.Context, key string, n int) (httpx.RateLimitResult, error) {
	res := httpx.RateLimitResult{Limit: int(rl.burst)}
	if n <= 0 {
		res.Allowed = true
		return res, nil
	}
	var tokens float64
	// Dos intentos: si la purga (PurgeRateLimits) se lleva la fila entre sembrarla y
	// gastarla, el UPDATE no encuentra nada; se vuelve a sembrar.
	for intento := 0; ; intento++ {
		if _, err := rl.q.Exec(ctx, rateLimitSembrarSQL, key, rl.burst); err != nil {
			return res, fmt.Errorf("pg: rate limit: %w", err)
		}
		err := rl.q.QueryRow(ctx, rateLimitNSQL, key, rl.rate, rl.burst, float64(n)).Scan(&res.Allowed, &tokens)
		if err == nil {
			break
		}
		if !errors.Is(err, pgx.ErrNoRows) || intento > 0 {
			return res, fmt.Errorf("pg: rate limit: %w", err)
		}
	}
	if res.Allowed {
		res.Remaining = int(tokens)
		return res, nil
	}
	res.RetryAfter = time.Duration((float64(n)-tokens)/rl.rate*float64(time.Second)) + time.Millisecond
	return res, nil
}

// Compile-time: *RateLimiter satisface httpx.RateLimiter.
var _ httpx.RateLimiter = (*RateLimiter)(nil)
