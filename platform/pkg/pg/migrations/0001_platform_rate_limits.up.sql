-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Estado del limitador token-bucket compartido entre réplicas (sobre el
-- datastore único PostgreSQL; sin Redis/Valkey). Cada fila es un bucket por
-- clave (p. ej. IP o sub+ruta). tokens puede ser negativo transitoriamente
-- (deuda) y se recupera con el tiempo según la tasa de recarga.

CREATE TABLE IF NOT EXISTS platform_rate_limits (
    bucket_key text PRIMARY KEY,
    tokens     double precision NOT NULL,
    updated_at timestamptz      NOT NULL DEFAULT now()
);

-- Índice para la purga periódica de buckets ociosos.
CREATE INDEX IF NOT EXISTS platform_rate_limits_updated_at_idx
    ON platform_rate_limits (updated_at);
