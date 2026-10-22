-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Estado del limitador token-bucket COMPARTIDO entre réplicas (pg.RateLimiter,
-- platform/pkg/pg). Copia literal de la migración de plataforma, como ya hacen
-- Sytue ID (0005), Cloud (0004) e Integraciones (0001): golang-migrate usa UNA
-- tabla de versiones por base de datos, así que el FS de plataforma no puede
-- aplicarse en paralelo. El DDL es IF NOT EXISTS e idempotente.
--
-- PARA QUÉ AQUÍ. El límite del enlace público de un cuaderno estaba en memoria,
-- o sea POR RÉPLICA: con `replicaCount: 2` y autoescalado por encima, el tope
-- real era el doble o más del que decía la configuración. Esa es justo la
-- superficie que puede visitar cualquiera sin sesión, y el ataque que tiene
-- enfrente es adivinar tokens a base de peticiones — el único límite que NO
-- puede multiplicarse por el número de instancias.

CREATE TABLE IF NOT EXISTS platform_rate_limits (
    bucket_key text PRIMARY KEY,
    tokens     double precision NOT NULL,
    updated_at timestamptz      NOT NULL DEFAULT now()
);

-- Índice para la purga periódica de buckets ociosos.
CREATE INDEX IF NOT EXISTS platform_rate_limits_updated_at_idx
    ON platform_rate_limits (updated_at);
