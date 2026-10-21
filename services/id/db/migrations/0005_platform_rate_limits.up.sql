-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Estado del limitador token-bucket compartido entre réplicas (pg.RateLimiter,
-- platform/pkg/pg). Copia literal de la migración de plataforma
-- platform/pkg/pg/migrations/0001_platform_rate_limits.up.sql: golang-migrate
-- usa UNA tabla de versiones por base de datos, así que el FS de plataforma no
-- puede aplicarse en paralelo (colisión de versiones); el DDL es IF NOT EXISTS
-- e idempotente. Lo usan los limitadores de /login y por-cuenta de Sytue ID.

CREATE TABLE IF NOT EXISTS platform_rate_limits (
    bucket_key text PRIMARY KEY,
    tokens     double precision NOT NULL,
    updated_at timestamptz      NOT NULL DEFAULT now()
);

-- Índice para la purga periódica de buckets ociosos.
CREATE INDEX IF NOT EXISTS platform_rate_limits_updated_at_idx
    ON platform_rate_limits (updated_at);
