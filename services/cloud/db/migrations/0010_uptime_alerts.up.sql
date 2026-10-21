-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Wave 2 — monitorización de uptime + alertas por evento (paridad DO/Coolify).
--
-- 1) uptime_checks: sonda HTTP periódica por proyecto. El prober (cross-réplica)
--    RECLAMA los vencidos (next_run_at <= now() AND enabled) con FOR UPDATE SKIP
--    LOCKED bajo pg.InMaintenanceTx (por eso la política *_maintenance, patrón
--    0002/0003), sondea con guard anti-SSRF y fija next_run_at = now()+interval.
--    Solo el ÚLTIMO estado se guarda aquí (sin histórico ilimitado en v1). Índice
--    parcial en (next_run_at) WHERE enabled para no escanear la tabla al reclamar.
--    target_url se valida anti-SSRF (domain.ValidateUptimeTarget) al fijarla.
-- 2) alert_policies: política de alerta por evento y canal (webhook saliente). El
--    consumer del bus (patrón DeployNotifier) resuelve las que casan por
--    (event, proyecto|org) y envía. project_id NULL = alcance organización.
-- 3) alert_deliveries: dedupe/idempotencia del envío por (policy_id, event_id) —
--    event_id es el id del evento del outbox, estable entre reentregas at-least-once.
--    Crecimiento continuo (un evento × política) ⇒ purga por sent_at (cron, como
--    events_outbox) + índice; política *_maintenance para la purga cross-tenant.
--
-- RLS deny-by-default por tenant en las tres tablas (patrón 0001); el worker fija
-- app.tenant (SetTenant) al reclamar/registrar/rutar bajo el tenant de la fila/evento.

CREATE TABLE uptime_checks (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant           text        NOT NULL,
    project_id       uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name             text        NOT NULL,
    target_url       text        NOT NULL,
    interval_seconds integer     NOT NULL DEFAULT 60 CHECK (interval_seconds >= 30 AND interval_seconds <= 86400),
    enabled          boolean     NOT NULL DEFAULT true,
    next_run_at      timestamptz NOT NULL DEFAULT now(),
    last_status      text        NOT NULL DEFAULT '' CHECK (last_status IN ('', 'up', 'down')),
    last_latency_ms  integer     NOT NULL DEFAULT 0,
    last_checked_at  timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now()
);

-- Cola del prober: vencidos y habilitados, en orden de vencimiento. Índice PARCIAL
-- para que la reclamación sea un Index Scan y no un Seq Scan de toda la tabla.
CREATE INDEX uptime_checks_due_idx ON uptime_checks (next_run_at) WHERE enabled;
-- Listado por proyecto (API CRUD).
CREATE INDEX uptime_checks_project_idx ON uptime_checks (tenant, project_id, created_at);

CREATE TABLE alert_policies (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    project_id  uuid        REFERENCES projects(id) ON DELETE CASCADE, -- NULL = toda la org
    event       text        NOT NULL CHECK (event IN ('deploy.failed', 'deploy.ready', 'uptime.down', 'uptime.up')),
    channel_url text        NOT NULL,
    enabled     boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Routing: policies habilitadas que casan por evento dentro del tenant. El
-- consumer filtra project_id = $proj OR project_id IS NULL en memoria/predicado.
CREATE INDEX alert_policies_route_idx ON alert_policies (tenant, event) WHERE enabled;

CREATE TABLE alert_deliveries (
    policy_id uuid        NOT NULL REFERENCES alert_policies(id) ON DELETE CASCADE,
    event_id  uuid        NOT NULL,
    tenant    text        NOT NULL,
    sent_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (policy_id, event_id)
);

-- Purga por antigüedad (retención corta, como el outbox): tabla de crecimiento
-- continuo. Índice para el DELETE ... WHERE sent_at < $1 del cron.
CREATE INDEX alert_deliveries_sent_idx ON alert_deliveries (sent_at);

-- RLS deny-by-default por tenant --------------------------------------------
ALTER TABLE uptime_checks    ENABLE ROW LEVEL SECURITY;
ALTER TABLE uptime_checks    FORCE  ROW LEVEL SECURITY;
ALTER TABLE alert_policies   ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_policies   FORCE  ROW LEVEL SECURITY;
ALTER TABLE alert_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_deliveries FORCE  ROW LEVEL SECURITY;

CREATE POLICY uptime_checks_tenant_isolation ON uptime_checks
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY alert_policies_tenant_isolation ON alert_policies
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY alert_deliveries_tenant_isolation ON alert_deliveries
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Mantenimiento (patrón 0002/0003): habilita SOLO bajo app.maintenance='on'
-- (pg.InMaintenanceTx) las operaciones cross-tenant sin BYPASSRLS:
--   - uptime_checks: reclamación del prober (SELECT ... FOR UPDATE SKIP LOCKED
--     + UPDATE del next_run_at) antes de conocer el tenant;
--   - alert_deliveries: purga por sent_at del cron.
-- Las transacciones de negocio nunca fijan el flag ⇒ aislamiento intacto.
CREATE POLICY uptime_checks_maintenance ON uptime_checks
    USING (current_setting('app.maintenance', true) = 'on');
CREATE POLICY alert_deliveries_maintenance ON alert_deliveries
    USING (current_setting('app.maintenance', true) = 'on');
