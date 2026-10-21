-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Metering real de cómputo (Wave 4 O4): compute_gb_hours emitido por el
-- reconciliador del controller.
--
-- 1) deployments.metered_at: instante de la última medición de cómputo de una
--    producción ready. NULL = aún sin medir (la query cae a ready_at/created_at,
--    con clamp de ventana). El reconciliador lo avanza a `now` en la MISMA
--    sentencia que lee la ventana (ClaimComputeUsage), así cada [metered_at, now)
--    se factura exactamente una vez —idempotente entre ciclos y reinicios— y el
--    FOR UPDATE ... SKIP LOCKED evita doble cobro entre controllers concurrentes.
-- 2) Índice parcial POR EXPRESIÓN que casa el predicado y el ORDER BY de
--    ClaimComputeUsage (COALESCE(metered_at, ready_at, created_at) sobre las
--    producciones ready): Index Scan acotado a las vencidas, no un Seq Scan de
--    todas las producciones cada ciclo a escala (cientos de miles).
--
-- NB: la agregación de uso por (tenant, metric, period) ya la sirve el índice
-- usage_metric_idx (tenant, metric, period_from) de 0001; NO se añade índice nuevo
-- sobre usage_records (evita duplicar). egress_gb/requests quedan DIFERIDOS: sin
-- pipeline de métricas de red/tráfico no hay serie que medir.

ALTER TABLE deployments ADD COLUMN metered_at timestamptz;

CREATE INDEX deployments_compute_meter_idx
    ON deployments (COALESCE(metered_at, ready_at, created_at))
    WHERE target = 'production' AND status = 'ready';
