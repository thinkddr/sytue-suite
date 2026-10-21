-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base de Sytue Cloud (PaaS): organizaciones, proyectos, builds,
-- deployments, dominios, variables/secretos, servicios gestionados y uso/
-- facturación. Todo el ESTADO en PostgreSQL (única fuente de verdad); los logs
-- de build y la cache viven en Object Storage S3 (no en la BD).
--
-- Multi-tenant: cada fila lleva `tenant` (slug de la organización). RLS
-- deny-by-default aísla por la variable de sesión `app.tenant` (la fija
-- pg.SetTenant en cada transacción). CONVENTIONS §9/§10.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Organizaciones (tenants). Una fila por organización; el slug es el namespace
-- lógico (org/<slug> en Sytue ID, tenant-<slug> en Kapsule, prefijo S3).
-- ---------------------------------------------------------------------------
CREATE TABLE organizations (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    -- tenant == slug; se duplica como `tenant` para que la RLS sea uniforme con
    -- el resto de tablas (la política compara `tenant` con app.tenant).
    tenant      text        NOT NULL UNIQUE,
    name        text        NOT NULL,
    plan        text        NOT NULL DEFAULT 'free' CHECK (plan IN ('free', 'pro', 'team')),
    region      text        NOT NULL DEFAULT 'fr-par',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz,
    CONSTRAINT organizations_slug_chk CHECK (tenant ~ '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$')
);

-- ---------------------------------------------------------------------------
-- Proyectos: unidad desplegable conectada a un repositorio git.
-- ---------------------------------------------------------------------------
CREATE TABLE projects (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant         text        NOT NULL,
    slug           text        NOT NULL,
    name           text        NOT NULL,
    repo_url       text        NOT NULL DEFAULT '',
    git_provider   text        NOT NULL DEFAULT 'git' CHECK (git_provider IN ('git', 'github', 'gitlab')),
    production_ref text        NOT NULL DEFAULT 'main',
    framework      text        NOT NULL DEFAULT 'auto'
                   CHECK (framework IN ('auto', 'astro', 'static', 'node', 'go', 'docker')),
    root_dir       text        NOT NULL DEFAULT '',
    build_command  text        NOT NULL DEFAULT '',
    output_dir     text        NOT NULL DEFAULT '',
    install_cmd    text        NOT NULL DEFAULT '',
    owner_sub      text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    deleted_at     timestamptz,
    CONSTRAINT projects_slug_chk CHECK (slug ~ '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$')
);

-- Slug de proyecto único por organización (entre vivos). El subdominio de la
-- app prod se deriva de este slug, por lo que debe ser único globalmente para
-- *.apps.sytue.eu; lo garantiza el índice global de abajo.
CREATE UNIQUE INDEX projects_unique_slug_per_tenant
    ON projects (tenant, slug) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX projects_unique_global_slug
    ON projects (slug) WHERE deleted_at IS NULL;
CREATE INDEX projects_tenant_idx ON projects (tenant) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Builds: construcción de imagen OCI de un commit (Kaniko rootless).
-- ---------------------------------------------------------------------------
CREATE TABLE builds (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    project_id   uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    commit_sha   text        NOT NULL DEFAULT '',
    git_ref      text        NOT NULL DEFAULT '',
    status       text        NOT NULL DEFAULT 'queued'
                 CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled')),
    framework    text        NOT NULL DEFAULT 'auto',
    image_ref    text        NOT NULL DEFAULT '',
    logs_key     text        NOT NULL DEFAULT '',
    triggered_by text        NOT NULL DEFAULT '',
    started_at   timestamptz,
    finished_at  timestamptz,
    error_msg    text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX builds_project_idx ON builds (tenant, project_id, created_at DESC);
-- Cola de builds pendientes (el builder toma con FOR UPDATE SKIP LOCKED).
CREATE INDEX builds_queued_idx ON builds (created_at) WHERE status = 'queued';

-- ---------------------------------------------------------------------------
-- Deployments: instancia desplegada de una build (producción o preview por PR).
-- ---------------------------------------------------------------------------
CREATE TABLE deployments (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    project_id  uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    build_id    uuid        REFERENCES builds (id) ON DELETE SET NULL,
    target      text        NOT NULL CHECK (target IN ('production', 'preview')),
    status      text        NOT NULL DEFAULT 'queued'
                CHECK (status IN ('queued', 'building', 'deploying', 'ready', 'error', 'canceled', 'retired')),
    commit_sha  text        NOT NULL DEFAULT '',
    git_ref     text        NOT NULL DEFAULT '',
    pr_number   integer,
    url         text        NOT NULL DEFAULT '',
    replicas    integer     NOT NULL DEFAULT 1 CHECK (replicas >= 0),
    created_by  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    ready_at    timestamptz,
    error_msg   text        NOT NULL DEFAULT '',
    CONSTRAINT deployments_preview_pr CHECK (target <> 'preview' OR pr_number IS NOT NULL)
);

CREATE INDEX deployments_project_idx ON deployments (tenant, project_id, created_at DESC);
-- Como mucho un deployment activo por (proyecto, PR) preview.
CREATE UNIQUE INDEX deployments_active_preview_idx
    ON deployments (tenant, project_id, pr_number)
    WHERE target = 'preview' AND status NOT IN ('retired', 'canceled', 'error');
-- Deployments que el controller debe reconciliar (no terminales).
CREATE INDEX deployments_reconcile_idx ON deployments (updated_at)
    WHERE status IN ('queued', 'building', 'deploying');

-- ---------------------------------------------------------------------------
-- Dominios personalizados (CNAME → ingress, TLS automático vía cert-manager).
-- ---------------------------------------------------------------------------
CREATE TABLE domains (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    project_id   uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    hostname     text        NOT NULL,
    status       text        NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'verified', 'active', 'failed')),
    verify_token text        NOT NULL DEFAULT '',
    is_primary   boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    verified_at  timestamptz
);

-- Un hostname pertenece a un único proyecto en toda la plataforma.
CREATE UNIQUE INDEX domains_unique_hostname ON domains (lower(hostname));
CREATE INDEX domains_project_idx ON domains (tenant, project_id);

-- ---------------------------------------------------------------------------
-- Variables de entorno / secretos. Para secretos, value_enc guarda el valor
-- cifrado en reposo (AES-256-GCM, clave de SECRET_ENC_KEY); nunca en claro.
-- ---------------------------------------------------------------------------
CREATE TABLE env_vars (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text        NOT NULL,
    project_id uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    key        text        NOT NULL,
    -- value_plain para variables no sensibles; value_enc (bytea) para secretos.
    value_plain text       NOT NULL DEFAULT '',
    value_enc  bytea,
    is_secret  boolean     NOT NULL DEFAULT false,
    scope      text        NOT NULL DEFAULT 'all' CHECK (scope IN ('all', 'production', 'preview')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT env_key_chk CHECK (key ~ '^[A-Z_][A-Z0-9_]*$'),
    CONSTRAINT env_secret_storage CHECK (
        (is_secret AND value_enc IS NOT NULL) OR (NOT is_secret)
    )
);

CREATE UNIQUE INDEX env_vars_unique_key ON env_vars (tenant, project_id, key, scope);

-- ---------------------------------------------------------------------------
-- Servicios gestionados (Sytue Postgres/PostgREST, Storage, Functions, Cron,
-- Auth). config_json es configuración pública (host, db, bucket, rest_url);
-- creds_enc guarda las credenciales sensibles cifradas (DSN, claves S3, API key)
-- que el controller inyecta en los deployments. Nunca se exponen en claro.
-- ---------------------------------------------------------------------------
CREATE TABLE managed_services (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    project_id  uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    kind        text        NOT NULL
                CHECK (kind IN ('postgres', 'storage', 'functions', 'cron', 'auth')),
    name        text        NOT NULL,
    status      text        NOT NULL DEFAULT 'provisioning'
                CHECK (status IN ('provisioning', 'ready', 'failed', 'deleting')),
    config_json jsonb       NOT NULL DEFAULT '{}'::jsonb,
    creds_enc   bytea,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE UNIQUE INDEX managed_services_unique_name
    ON managed_services (tenant, project_id, kind, name) WHERE deleted_at IS NULL;
CREATE INDEX managed_services_project_idx
    ON managed_services (tenant, project_id) WHERE deleted_at IS NULL;
-- Servicios que el controller debe aprovisionar/eliminar.
CREATE INDEX managed_services_pending_idx ON managed_services (updated_at)
    WHERE status IN ('provisioning', 'deleting');

-- ---------------------------------------------------------------------------
-- Uso / facturación: registros agregados por organización y periodo.
-- ---------------------------------------------------------------------------
CREATE TABLE usage_records (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    project_id  uuid        REFERENCES projects (id) ON DELETE SET NULL,
    metric      text        NOT NULL,
    quantity    double precision NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    unit        text        NOT NULL DEFAULT '',
    period_from timestamptz NOT NULL,
    period_to   timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT usage_period_chk CHECK (period_to >= period_from)
);

CREATE INDEX usage_tenant_period_idx ON usage_records (tenant, period_from, period_to);
CREATE INDEX usage_metric_idx ON usage_records (tenant, metric, period_from);

-- ---------------------------------------------------------------------------
-- Webhooks git entrantes (deduplicación + auditoría). delivery_id evita
-- procesar dos veces el mismo evento (idempotencia).
-- ---------------------------------------------------------------------------
CREATE TABLE git_webhooks (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    project_id  uuid        REFERENCES projects (id) ON DELETE CASCADE,
    provider    text        NOT NULL,
    delivery_id text        NOT NULL,
    event_type  text        NOT NULL DEFAULT '',
    payload     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    received_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX git_webhooks_dedup_idx
    ON git_webhooks (provider, delivery_id) WHERE delivery_id <> '';

-- ---------------------------------------------------------------------------
-- Outbox de eventos (sobre PostgreSQL; sin Redis/Valkey). DDL compartido de
-- platform/pkg/events, replicado aquí porque cada servicio gestiona su esquema.
-- ---------------------------------------------------------------------------
CREATE TABLE events_outbox (
    id           uuid        PRIMARY KEY,
    channel      text        NOT NULL,
    type         text        NOT NULL,
    tenant       text        NOT NULL DEFAULT '',
    payload      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    attempts     int         NOT NULL DEFAULT 0,
    last_error   text
);

CREATE INDEX events_outbox_unprocessed_idx
    ON events_outbox (created_at)
    WHERE processed_at IS NULL;

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por tenant deny-by-default (CONVENTIONS §10).
-- Las políticas comparan `tenant` con current_setting('app.tenant', true), que
-- fija pg.SetTenant LOCAL a cada transacción de negocio.
-- ---------------------------------------------------------------------------
ALTER TABLE organizations    ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects         ENABLE ROW LEVEL SECURITY;
ALTER TABLE builds           ENABLE ROW LEVEL SECURITY;
ALTER TABLE deployments      ENABLE ROW LEVEL SECURITY;
ALTER TABLE domains          ENABLE ROW LEVEL SECURITY;
ALTER TABLE env_vars         ENABLE ROW LEVEL SECURITY;
ALTER TABLE managed_services ENABLE ROW LEVEL SECURITY;
ALTER TABLE usage_records    ENABLE ROW LEVEL SECURITY;
ALTER TABLE git_webhooks     ENABLE ROW LEVEL SECURITY;

ALTER TABLE organizations    FORCE ROW LEVEL SECURITY;
ALTER TABLE projects         FORCE ROW LEVEL SECURITY;
ALTER TABLE builds           FORCE ROW LEVEL SECURITY;
ALTER TABLE deployments      FORCE ROW LEVEL SECURITY;
ALTER TABLE domains          FORCE ROW LEVEL SECURITY;
ALTER TABLE env_vars         FORCE ROW LEVEL SECURITY;
ALTER TABLE managed_services FORCE ROW LEVEL SECURITY;
ALTER TABLE usage_records    FORCE ROW LEVEL SECURITY;
ALTER TABLE git_webhooks     FORCE ROW LEVEL SECURITY;

CREATE POLICY organizations_tenant_isolation ON organizations
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY projects_tenant_isolation ON projects
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY builds_tenant_isolation ON builds
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY deployments_tenant_isolation ON deployments
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY domains_tenant_isolation ON domains
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY env_vars_tenant_isolation ON env_vars
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY managed_services_tenant_isolation ON managed_services
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY usage_records_tenant_isolation ON usage_records
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY git_webhooks_tenant_isolation ON git_webhooks
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- ---------------------------------------------------------------------------
-- Resolución de webhooks SIN tenant en el contexto: el endpoint público de
-- webhooks resuelve el proyecto/tenant a partir de la URL del repo. Función
-- SECURITY DEFINER acotada que devuelve el tenant del proyecto vivo cuyo
-- repo_url coincide (case-insensitive), para luego fijar app.tenant y operar
-- bajo RLS. Devuelve '' si no hay coincidencia.
-- ---------------------------------------------------------------------------
CREATE FUNCTION cloud_resolve_repo_tenant(p_repo text)
RETURNS text
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
    SELECT tenant FROM projects
    WHERE deleted_at IS NULL AND lower(repo_url) = lower(p_repo)
    LIMIT 1
$$;

REVOKE ALL ON FUNCTION cloud_resolve_repo_tenant(text) FROM PUBLIC;
