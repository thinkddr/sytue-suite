-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Pages, núcleo (PAGES-ENCARGO §5): sitios, slugs retirados, versiones con
-- nombre, publicaciones, redirecciones y recursos; más el ALTCHA de un solo uso, el
-- limitador compartido y el outbox de eventos.
--
-- Reglas de TODA tabla con `tenant`:
--   · RLS ENABLE + FORCE (el rol de la aplicación es el propietario: sin FORCE la
--     política ni se evalúa) con tres políticas: aislamiento (`app.tenant`), fusión de
--     organizaciones (`app.merge`, la reconoce el preflight de Sytue ID) y
--     mantenimiento (`app.maintenance`: mapa, barridos y la CLI de suspensión).
--   · Claves únicas por `site_id` o `id`, NUNCA por tenant: la fusión hace
--     `UPDATE … SET tenant` y chocaría.
-- `app.pages_publico` (solo lectura) abre `sites` y `retired_slugs` a la resolución
-- Host → sitio y a la disponibilidad de slugs, sin sesión. Lo fija UNA función del
-- store (`enPublico`).

-- ---------------------------------------------------------------------------
-- Sitios
-- ---------------------------------------------------------------------------
CREATE TABLE sites (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant             text NOT NULL,
    -- El slug es de la WEB, no de la organización: único en toda la plataforma
    -- porque es su dirección <slug>.<PAGES_SITES_DOMAIN>.
    slug               text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'),
    name               text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    template           text NOT NULL DEFAULT '',
    -- suspended: por abuso, desde la CLI (`pages suspender`).
    status             text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    suspended_reason   text NOT NULL DEFAULT '',
    draft              jsonb NOT NULL,
    draft_rev          bigint NOT NULL DEFAULT 1,
    draft_updated_by   text NOT NULL DEFAULT '',
    draft_updated_at   timestamptz NOT NULL DEFAULT now(),
    -- publications.number vigente; NULL = sin publicar.
    published_number   integer,
    -- NULL = <slug>.<PAGES_SITES_DOMAIN>.
    canonical_host     text,
    -- DatosLegales (§7.2).
    legal              jsonb NOT NULL DEFAULT '{}'::jsonb,
    analytics_enabled  boolean NOT NULL DEFAULT true,
    analytics_site_id  text NOT NULL DEFAULT '',
    -- Orígenes y esquemas desde los que el código EXPORTADO llama a /_pages/*
    -- (§9.5): https:// sin ruta, y esquemas propios de las apps nativas.
    export_origins     text[] NOT NULL DEFAULT '{}' CHECK (cardinality(export_origins) <= 10),
    export_app_schemes text[] NOT NULL DEFAULT '{}' CHECK (cardinality(export_app_schemes) <= 5),
    created_by         text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    -- Borrado blando, restaurable 30 días.
    deleted_at         timestamptz
);
CREATE INDEX sites_por_tenant ON sites (tenant, created_at DESC) WHERE deleted_at IS NULL;

-- Slug abandonado (al renombrar o en el borrado duro): solo su tenant lo recupera
-- durante un año, y mientras exista su sitio, su host redirige 301 al canónico.
CREATE TABLE retired_slugs (
    slug    text PRIMARY KEY,
    tenant  text NOT NULL,
    site_id uuid REFERENCES sites (id) ON DELETE SET NULL,
    until   timestamptz NOT NULL
);

-- Versiones con nombre del diseño (patrón de doc_versions de Docs).
CREATE TABLE site_versions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text NOT NULL,
    site_id    uuid NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    name       text NOT NULL DEFAULT '' CHECK (length(name) <= 120),
    kind       text NOT NULL CHECK (kind IN ('manual', 'publish', 'preview', 'export', 'before_ai', 'before_restore')),
    ir         jsonb NOT NULL,
    draft_rev  bigint NOT NULL,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX site_versions_por_sitio ON site_versions (tenant, site_id, created_at DESC);

CREATE TABLE publications (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant           text NOT NULL,
    site_id          uuid NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    kind             text NOT NULL CHECK (kind IN ('live', 'preview')),
    -- live: 1, 2, 3… por sitio; preview: NULL.
    number           integer,
    -- sha256(token)[:32] en hex (solo preview): el token no se guarda en claro.
    preview_key      text CHECK (preview_key ~ '^[0-9a-f]{32}$'),
    -- preview: +7 días.
    expires_at       timestamptz,
    version_id       uuid NOT NULL REFERENCES site_versions (id),
    files            integer NOT NULL,
    bytes            bigint NOT NULL,
    renderer_version text NOT NULL,
    -- {store, legal, hosts}: sha256 de esas partes de DatosRender (`pending`, §6.2).
    data_hash        jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by       text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (site_id, number),
    CHECK ((kind = 'live') = (number IS NOT NULL)),
    CHECK ((kind = 'preview') = (preview_key IS NOT NULL))
);

-- 301 por cambio de slug de una página.
CREATE TABLE redirects (
    tenant     text NOT NULL,
    site_id    uuid NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    from_path  text NOT NULL CHECK (from_path ~ '^/[a-z]{2}/[a-z0-9/-]*$'),
    to_path    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, from_path)
);

CREATE TABLE assets (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text NOT NULL,
    site_id    uuid NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    -- 32 hex del sha256 de los bytes normalizados.
    file       text NOT NULL CHECK (file ~ '^[0-9a-f]{32}$'),
    ext        text NOT NULL CHECK (ext IN ('jpg', 'png', 'pdf')),
    -- Un pdf: 0 × 0.
    width      integer NOT NULL DEFAULT 0,
    height     integer NOT NULL DEFAULT 0,
    bytes      bigint NOT NULL,
    -- Anchos generados, SIEMPRE con el propio: {300} o {480,960,1600,2400}.
    variants   integer[] NOT NULL DEFAULT '{}',
    origin     text NOT NULL CHECK (origin IN ('upload', 'drive', 'photos')),
    -- En un pdf, el nombre de la descarga.
    name       text NOT NULL DEFAULT '' CHECK (length(name) <= 200),
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (site_id, file)
);

-- ---------------------------------------------------------------------------
-- Aislamiento por organización, fusión y mantenimiento.
-- ---------------------------------------------------------------------------
ALTER TABLE sites         ENABLE ROW LEVEL SECURITY;
ALTER TABLE sites         FORCE ROW LEVEL SECURITY;
ALTER TABLE retired_slugs ENABLE ROW LEVEL SECURITY;
ALTER TABLE retired_slugs FORCE ROW LEVEL SECURITY;
ALTER TABLE site_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE site_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE publications  ENABLE ROW LEVEL SECURITY;
ALTER TABLE publications  FORCE ROW LEVEL SECURITY;
ALTER TABLE redirects     ENABLE ROW LEVEL SECURITY;
ALTER TABLE redirects     FORCE ROW LEVEL SECURITY;
ALTER TABLE assets        ENABLE ROW LEVEL SECURITY;
ALTER TABLE assets        FORCE ROW LEVEL SECURITY;

CREATE POLICY sites_tenant_isolation ON sites
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY sites_platform_merge ON sites
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY sites_mantenimiento ON sites
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
CREATE POLICY sites_publico ON sites FOR SELECT
    USING (current_setting('app.pages_publico', true) = 'on');

CREATE POLICY retired_slugs_tenant_isolation ON retired_slugs
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY retired_slugs_platform_merge ON retired_slugs
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY retired_slugs_mantenimiento ON retired_slugs
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
CREATE POLICY retired_slugs_publico ON retired_slugs FOR SELECT
    USING (current_setting('app.pages_publico', true) = 'on');

CREATE POLICY site_versions_tenant_isolation ON site_versions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY site_versions_platform_merge ON site_versions
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY site_versions_mantenimiento ON site_versions
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY publications_tenant_isolation ON publications
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY publications_platform_merge ON publications
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY publications_mantenimiento ON publications
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY redirects_tenant_isolation ON redirects
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY redirects_platform_merge ON redirects
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY redirects_mantenimiento ON redirects
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY assets_tenant_isolation ON assets
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY assets_platform_merge ON assets
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY assets_mantenimiento ON assets
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- ---------------------------------------------------------------------------
-- ALTCHA de un solo uso: SIN tenant ni RLS a propósito. Es la firma anónima de
-- la verja pública de las webs (checkout, formularios, denuncias), no un dato de
-- ninguna organización. INSERT … ON CONFLICT DO NOTHING y barrido de caducadas:
-- vale con cualquier número de réplicas, a diferencia de la caché en memoria.
-- ---------------------------------------------------------------------------
CREATE TABLE altcha_used (
    signature_hash text PRIMARY KEY,
    expires_at     timestamptz NOT NULL
);
CREATE INDEX altcha_used_caducadas ON altcha_used (expires_at);

-- ---------------------------------------------------------------------------
-- Limitador token-bucket COMPARTIDO entre réplicas (pg.RateLimiter): copia literal
-- de la migración de plataforma (golang-migrate usa UNA tabla de versiones por
-- base). Los cubos por sitio de /_pages/* y los de los webhooks viven aquí, y el
-- middleware es FAIL-CLOSED: sin la tabla, todo eso contestaría 503.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS platform_rate_limits (
    bucket_key text PRIMARY KEY,
    tokens     double precision NOT NULL,
    updated_at timestamptz      NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS platform_rate_limits_updated_at_idx
    ON platform_rate_limits (updated_at);

-- ---------------------------------------------------------------------------
-- Outbox de eventos (platform/pkg/events.OutboxTableDDL, con locked_until): los
-- efectos de pedidos, pagos y formularios, con solo ids en el payload.
--
-- Lleva `tenant`, así que lleva política de FUSIÓN (sin ella el preflight de Sytue
-- ID abortaría todas las fusiones de la suite), pero NO lleva RLS: la recorre el
-- despachador de todos los tenants a la vez, y con FORCE dejaría de ver las filas.
-- Es la excepción declarada en ExigirRLSForzada.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS events_outbox (
    id           uuid PRIMARY KEY,
    channel      text        NOT NULL,
    type         text        NOT NULL,
    tenant       text        NOT NULL DEFAULT '',
    payload      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    attempts     int         NOT NULL DEFAULT 0,
    last_error   text,
    locked_until timestamptz
);
CREATE INDEX IF NOT EXISTS events_outbox_unprocessed_idx
    ON events_outbox (created_at)
    WHERE processed_at IS NULL;

CREATE POLICY events_outbox_platform_merge ON events_outbox
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
