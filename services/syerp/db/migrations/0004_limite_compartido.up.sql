-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Estado del limitador token-bucket COMPARTIDO entre réplicas (pg.RateLimiter,
-- platform/pkg/pg). Copia literal de la migración de plataforma, como ya hacen
-- Sytue ID (0005), Cloud (0004), Integraciones (0001) y Notepads (0007):
-- golang-migrate usa UNA tabla de versiones por base de datos, así que el FS de
-- plataforma no puede aplicarse en paralelo. El DDL es IF NOT EXISTS e
-- idempotente.
--
-- PARA QUÉ AQUÍ, y cómo se descubrió que faltaba. El webhook de Stripe es la
-- ÚNICA superficie de este servicio que puede visitar cualquiera sin sesión, así
-- que su límite es el que no puede multiplicarse por el número de réplicas: un
-- cubo en memoria vive en cada pod y con autoescalado el tope real crece justo
-- cuando hay carga. Por eso usa el limitador compartido... que consulta esta
-- tabla, que este servicio no creaba.
--
-- El resultado NO era un webhook sin límite: el middleware está configurado
-- fail-closed, así que cada petición de Stripe se iba en un 503 y el conector
-- estaba muerto de principio a fin. No lo vio ningún test —los unitarios no
-- tocan la base y los de integración no montan el router— ni el linter ni el
-- render de Helm. Apareció al levantar el contenedor y hacer un `curl`.

CREATE TABLE IF NOT EXISTS platform_rate_limits (
    bucket_key text PRIMARY KEY,
    tokens     double precision NOT NULL,
    updated_at timestamptz      NOT NULL DEFAULT now()
);

-- Índice para la purga periódica de buckets ociosos.
CREATE INDEX IF NOT EXISTS platform_rate_limits_updated_at_idx
    ON platform_rate_limits (updated_at);
