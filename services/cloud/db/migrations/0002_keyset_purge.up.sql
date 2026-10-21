-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Escala de listados y purga (CONVENTIONS §0.8: keyset, sin OFFSET; retención).
--
-- 1) Índices compuestos (tenant, project_id, created_at DESC, id DESC) que
--    sirven la paginación KEYSET de builds/deployments: el predicado
--    (created_at, id) < ($ts, $id) con ORDER BY created_at DESC, id DESC
--    recorre el índice sin re-ordenar. Sustituyen a los índices previos
--    (prefijo estricto, redundantes).
-- 2) Índices parciales para el cron de purga (Index Scan, no Seq Scan; mismo
--    criterio que el cron de id, ver fbf7e87).
-- 3) Políticas PERMISSIVE de mantenimiento (patrón hub/mail): habilitan las
--    operaciones cross-tenant del cron de purga/rollup SOLO cuando la sesión
--    fija `app.maintenance = 'on'` (pg.InMaintenanceTx). Las transacciones de
--    negocio nunca fijan ese flag, así que el aislamiento por tenant queda
--    intacto.

-- 1) Keyset -------------------------------------------------------------------
CREATE INDEX builds_project_created_id_idx
    ON builds (tenant, project_id, created_at DESC, id DESC);
DROP INDEX builds_project_idx;

CREATE INDEX deployments_project_created_id_idx
    ON deployments (tenant, project_id, created_at DESC, id DESC);
DROP INDEX deployments_project_idx;

-- 2) Purga --------------------------------------------------------------------
-- Builds terminadas (candidatas a purga por antigüedad).
CREATE INDEX builds_terminal_created_idx ON builds (created_at)
    WHERE status IN ('succeeded', 'failed', 'canceled');
-- Previews retiradas (candidatas a purga a los 7 días).
CREATE INDEX deployments_retired_preview_idx ON deployments (updated_at)
    WHERE target = 'preview' AND status = 'retired';
-- Webhooks por antigüedad (retención 30 días).
CREATE INDEX git_webhooks_received_idx ON git_webhooks (received_at);
-- Eventos del outbox ya procesados (retención 7 días).
CREATE INDEX events_outbox_processed_idx ON events_outbox (processed_at)
    WHERE processed_at IS NOT NULL;
-- Detalle de uso por inicio de periodo (rollup mensual + purga a los 90 días).
CREATE INDEX usage_records_period_from_idx ON usage_records (period_from);

-- 3) Mantenimiento ------------------------------------------------------------
-- events_outbox no tiene RLS (no lleva política); el resto de tablas que toca
-- el cron sí. USING sin WITH CHECK: la misma expresión gobierna también los
-- INSERT del rollup mensual de usage_records.
CREATE POLICY builds_maintenance ON builds
    USING (current_setting('app.maintenance', true) = 'on');
CREATE POLICY deployments_maintenance ON deployments
    USING (current_setting('app.maintenance', true) = 'on');
CREATE POLICY git_webhooks_maintenance ON git_webhooks
    USING (current_setting('app.maintenance', true) = 'on');
CREATE POLICY usage_records_maintenance ON usage_records
    USING (current_setting('app.maintenance', true) = 'on');
