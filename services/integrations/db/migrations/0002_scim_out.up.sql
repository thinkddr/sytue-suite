-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- SCIM 2.0 SALIENTE (OLA 2B): aprovisionar/desaprovisionar usuarios+grupos EN el
-- SaaS de terceros por reconcile-por-diff. Dos tablas por-org (RLS deny-by-default):
--   - scim_map  : el mapeo Sytue↔remoto (id remoto + hash del último estado
--                 empujado) que hace idempotente el diff y adopta preexistentes.
--   - sync_runs : el historial de ejecuciones de sincronización (contadores +
--                 estado) que alimenta GET /instances/{id}/sync-runs.
--
-- Mismo modelo de aislamiento que 0001: org_id (slug), RLS ENABLE+FORCE comparando
-- con current_setting('app.tenant', true), y el rol NO superusuario integrations_app
-- (el store hace SET LOCAL ROLE en cada transacción por-org).

-- ---------------------------------------------------------------------------
-- scim_map — un renglón por entidad de Sytue (usuario o grupo) aprovisionada en
-- una instancia SCIM. remote_id es el id del recurso en el SaaS; state_hash es el
-- hash del último payload empujado (si el hash deseado coincide, el diff no toca
-- nada ⇒ idempotencia). PK (instance_id, kind, sytue_id): una entidad de Sytue
-- mapea a UN recurso remoto por instancia.
-- ---------------------------------------------------------------------------
CREATE TABLE scim_map (
    instance_id uuid        NOT NULL REFERENCES app_instances (id) ON DELETE CASCADE,
    org_id      text        NOT NULL,
    kind        text        NOT NULL CHECK (kind IN ('user', 'group')),
    sytue_id    text        NOT NULL,
    remote_id   text        NOT NULL,
    state_hash  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (instance_id, kind, sytue_id)
);

-- ---------------------------------------------------------------------------
-- sync_runs — historial de reconciliaciones de una instancia. finished_at NULL
-- mientras corre; status running|ok|error. Los contadores resumen el diff.
-- ---------------------------------------------------------------------------
CREATE TABLE sync_runs (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id       uuid        NOT NULL REFERENCES app_instances (id) ON DELETE CASCADE,
    org_id            text        NOT NULL,
    status            text        NOT NULL DEFAULT 'running'
                                  CHECK (status IN ('running', 'ok', 'error')),
    created_count     int         NOT NULL DEFAULT 0,
    updated_count     int         NOT NULL DEFAULT 0,
    deactivated_count int         NOT NULL DEFAULT 0,
    error_count       int         NOT NULL DEFAULT 0,
    started_at        timestamptz NOT NULL DEFAULT now(),
    finished_at       timestamptz
);

-- Historial por instancia, más reciente primero (GET /sync-runs, keyset).
CREATE INDEX sync_runs_instance_started ON sync_runs (instance_id, started_at DESC, id DESC);

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por org deny-by-default (igual que 0001).
-- ---------------------------------------------------------------------------
ALTER TABLE scim_map  ENABLE ROW LEVEL SECURITY;
ALTER TABLE sync_runs ENABLE ROW LEVEL SECURITY;

ALTER TABLE scim_map  FORCE ROW LEVEL SECURITY;
ALTER TABLE sync_runs FORCE ROW LEVEL SECURITY;

CREATE POLICY scim_map_org_isolation ON scim_map
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));
CREATE POLICY sync_runs_org_isolation ON sync_runs
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));

-- Las tablas nuevas necesitan su propio GRANT: el de 0001 solo alcanzó a las
-- tablas existentes en aquel momento. Sin esto, integrations_app no podría operar.
GRANT SELECT, INSERT, UPDATE, DELETE ON scim_map  TO integrations_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON sync_runs TO integrations_app;
