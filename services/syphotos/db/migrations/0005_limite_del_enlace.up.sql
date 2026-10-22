-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Estado del limitador token-bucket COMPARTIDO entre réplicas (pg.RateLimiter,
-- platform/pkg/pg). Copia literal de la migración de plataforma, como ya hacen
-- Sytue ID, Cloud, Integraciones y Notes: golang-migrate usa UNA tabla de
-- versiones por base de datos, así que el FS de plataforma no puede aplicarse
-- en paralelo. El DDL es IF NOT EXISTS e idempotente.
--
-- PARA QUÉ AQUÍ. El enlace de un álbum compartido es la ÚNICA superficie de
-- este servicio que puede visitar cualquiera sin sesión, y lo que tiene
-- enfrente es adivinar tokens a base de peticiones. Un tope en memoria es un
-- tope POR RÉPLICA: con dos réplicas el límite real es el doble, y crece justo
-- cuando hay carga. Es el único límite que no puede permitirse eso.

CREATE TABLE IF NOT EXISTS platform_rate_limits (
    bucket_key text PRIMARY KEY,
    tokens     double precision NOT NULL,
    updated_at timestamptz      NOT NULL DEFAULT now()
);

-- Índice para la purga periódica de buckets ociosos.
CREATE INDEX IF NOT EXISTS platform_rate_limits_updated_at_idx
    ON platform_rate_limits (updated_at);
