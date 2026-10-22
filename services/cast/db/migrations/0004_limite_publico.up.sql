-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Estado del limitador token-bucket COMPARTIDO entre réplicas (pg.RateLimiter,
-- platform/pkg/pg). Copia literal de la migración de plataforma, como en Sytue ID
-- (0005), Cloud (0004), Integraciones (0001) y Notes (0007): golang-migrate usa
-- UNA tabla de versiones por base, así que el FS de plataforma no puede
-- aplicarse aparte. El DDL es IF NOT EXISTS e idempotente.
--
-- PARA QUÉ AQUÍ. `/public`, `/api/v1/public`, `/media` y `/ws` limitan por IP con
-- ese cubo, y el middleware es FAIL-CLOSED: sin la tabla, todo lo anónimo y toda
-- la reproducción respondían 503 «Limitador no disponible» en una base recién
-- creada.

CREATE TABLE IF NOT EXISTS platform_rate_limits (
    bucket_key text PRIMARY KEY,
    tokens     double precision NOT NULL,
    updated_at timestamptz      NOT NULL DEFAULT now()
);

-- Índice para la purga periódica de buckets ociosos.
CREATE INDEX IF NOT EXISTS platform_rate_limits_updated_at_idx
    ON platform_rate_limits (updated_at);
