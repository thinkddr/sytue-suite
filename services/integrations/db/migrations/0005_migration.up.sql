-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- MIGRACIÓN DE CONTENIDO desde Microsoft 365 / Google Workspace hacia Sytue (el
-- on-ramp de adopción). Dos tablas por-org (RLS deny-by-default), mismo modelo de
-- aislamiento que 0001/0002:
--   - migration_jobs : un trabajo de importación (origen, dominio, usuario objetivo,
--                      estado, contadores y CURSOR de reanudación).
--   - migration_map  : id de origen → id en Sytue por trabajo. Es a la vez la
--                      garantía de IDEMPOTENCIA (un id ya mapeado no se reescribe
--                      nunca) y el INFORME (skip_reason explica cada omisión).
--
-- El correo y el calendario NO entran aquí a propósito (hilos, adjuntos,
-- recurrencias y zonas horarias son otra ola): el CHECK de `domain` lo hace
-- explícito en vez de aceptar en silencio algo que no se migra.

-- ---------------------------------------------------------------------------
-- migration_jobs — un trabajo por (usuario objetivo, origen, dominio). El cursor
-- es un puntero OPACO específico del dominio (cola BFS de carpetas en drive,
-- pageToken en contacts): reanudar = retomar el recorrido donde se quedó, sin
-- volver a listar el árbol entero.
--
-- dest_token_enc es el token con el que se ESCRIBE en Sytue (Drive/Contacts),
-- sellado con la KEK (AES-256-GCM, igual que instance_secrets). Es el token del
-- admin que lanza la migración: el contenido aterriza en SU cuenta. Nunca se
-- serializa a la API.
-- ---------------------------------------------------------------------------
CREATE TABLE migration_jobs (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         text        NOT NULL,
    instance_id    uuid        NOT NULL REFERENCES app_instances (id) ON DELETE CASCADE,
    -- subject: el `sub` del JWT del usuario en cuya cuenta de Sytue aterriza todo.
    subject        text        NOT NULL,
    -- source_user: la identidad EN EL ORIGEN de la que se lee (UPN de M365 o
    -- email del Workspace). Con auth de app (Graph) o delegación de dominio (GWS)
    -- no hay "/me": hay que decir de quién se copia.
    source_user    text        NOT NULL,
    source         text        NOT NULL CHECK (source IN ('m365', 'google')),
    domain         text        NOT NULL CHECK (domain IN ('drive', 'contacts')),
    status         text        NOT NULL DEFAULT 'queued'
                               CHECK (status IN ('queued', 'running', 'done', 'error', 'canceled')),
    cursor         text        NOT NULL DEFAULT '',
    dest_token_enc text        NOT NULL DEFAULT '',
    migrated_count int         NOT NULL DEFAULT 0,
    skipped_count  int         NOT NULL DEFAULT 0,
    error_count    int         NOT NULL DEFAULT 0,
    bytes_copied   bigint      NOT NULL DEFAULT 0,
    last_error     text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    started_at     timestamptz,
    finished_at    timestamptz
);

-- Listado del panel de migraciones de la org, más reciente primero (keyset).
CREATE INDEX migration_jobs_org_created ON migration_jobs (org_id, created_at DESC, id DESC);

-- ---------------------------------------------------------------------------
-- migration_map — un renglón por entidad VISTA en el origen. sytue_id poblado ⇒
-- se copió; skip_reason poblado ⇒ se omitió y aquí está el porqué (nada se cuenta
-- como éxito sin haberse copiado). item_hash es el hash del estado de origen.
-- PK (job_id, source_id): idempotencia — reejecutar el trabajo no duplica.
-- ---------------------------------------------------------------------------
CREATE TABLE migration_map (
    job_id      uuid        NOT NULL REFERENCES migration_jobs (id) ON DELETE CASCADE,
    org_id      text        NOT NULL,
    source_id   text        NOT NULL,
    kind        text        NOT NULL DEFAULT '' CHECK (kind IN ('', 'folder', 'file', 'contact')),
    sytue_id    text        NOT NULL DEFAULT '',
    item_hash   text        NOT NULL DEFAULT '',
    path        text        NOT NULL DEFAULT '',
    skip_reason text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, source_id)
);

-- Informe de omisiones de un trabajo (GET /migrations/{id}), sin escanear el mapa
-- entero: parcial sobre las filas que SÍ tienen motivo.
CREATE INDEX migration_map_skipped ON migration_map (job_id, source_id) WHERE skip_reason <> '';

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por org deny-by-default (igual que 0001/0002).
-- ---------------------------------------------------------------------------
ALTER TABLE migration_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE migration_map  ENABLE ROW LEVEL SECURITY;

ALTER TABLE migration_jobs FORCE ROW LEVEL SECURITY;
ALTER TABLE migration_map  FORCE ROW LEVEL SECURITY;

CREATE POLICY migration_jobs_org_isolation ON migration_jobs
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));
CREATE POLICY migration_map_org_isolation ON migration_map
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));

-- Las tablas nuevas necesitan su propio GRANT (el de 0001 solo alcanzó a las de
-- entonces). Sin esto, integrations_app no podría operar bajo SET LOCAL ROLE.
GRANT SELECT, INSERT, UPDATE, DELETE ON migration_jobs TO integrations_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON migration_map  TO integrations_app;
