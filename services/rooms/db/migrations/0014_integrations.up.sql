-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- INTEGRACIONES (Ola 15 · paridad Mattermost): webhooks ENTRANTES, webhooks
-- SALIENTES (SSRF-safe) y BOTS. Tres identidades/mecanismos para que sistemas
-- externos publiquen en canales o reaccionen a mensajes.
--
-- SEGURIDAD:
--   * Los tokens NUNCA se guardan en claro: se persiste su SHA-256 (token_hash).
--     El token en claro solo se devuelve UNA vez, al crear/rotar. La verificación
--     rehashea el token entrante y compara con la columna (índice único cubre la
--     búsqueda por hash sin escanear).
--   * RLS deny-by-default por tenant (como el resto del esquema). Además, una
--     política PERMISSIVE de mantenimiento (app.maintenance='on') permite la
--     búsqueda CROSS-TENANT por token en dos superficies SIN sesión de usuario:
--       - el endpoint público POST /hooks/{token} (entrante; el token ES la
--         autorización, no hay JWT ni tenant en el contexto todavía);
--       - el middleware de auth de bot (Bearer bot_… ⇒ identidad bot:<id>).
--     El camino de gestión (crear/listar/borrar, InTenantTx) usa la política por
--     tenant. Mismo patrón que scheduled_messages (mig. 0010) y el MX de Mail.

-- ---------------------------------------------------------------------------
-- Webhooks ENTRANTES: un token opaco ligado a un canal. POST /hooks/{token}
-- publica un mensaje en ese canal como autor sintético hook:<id> (badge BOT).
-- ---------------------------------------------------------------------------
CREATE TABLE incoming_webhooks (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    channel_id  uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    token_hash  text        NOT NULL,
    name        text        NOT NULL DEFAULT '',
    icon        text        NOT NULL DEFAULT '',
    created_by  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Búsqueda por token en el endpoint público (cross-tenant): el hash es único a
-- nivel global y su índice cubre el lookup por igualdad sin escaneo.
CREATE UNIQUE INDEX incoming_webhooks_token_hash ON incoming_webhooks (token_hash);
-- Gestión: listar los webhooks de un canal (admin).
CREATE INDEX incoming_webhooks_by_channel ON incoming_webhooks (tenant, channel_id);

-- ---------------------------------------------------------------------------
-- Webhooks SALIENTES: al crearse un mensaje de USUARIO que casa (canal y/o
-- palabra disparadora), un consumidor del outbox hace un POST SSRF-safe a
-- callback_url con `token` de verificación en la cabecera. channel_id NULL = no
-- acotado a un canal (exige entonces palabras disparadoras; lo valida el servicio).
-- ---------------------------------------------------------------------------
CREATE TABLE outgoing_webhooks (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    channel_id    uuid        REFERENCES channels (id) ON DELETE CASCADE,
    trigger_words text[]      NOT NULL DEFAULT '{}',
    callback_url  text        NOT NULL,
    -- Token de VERIFICACIÓN que viaja en la cabecera del POST para que el receptor
    -- confirme el origen; no autentica hacia nosotros, así que se guarda en claro
    -- (se envía en claro de todos modos). Se puede mostrar en la gestión.
    token         text        NOT NULL DEFAULT '',
    created_by    text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Coincidencia por tenant (el consumidor filtra channel_id NULL o = canal y las
-- palabras en Go). Los salientes son pocos por tenant (los crea un admin).
CREATE INDEX outgoing_webhooks_by_tenant ON outgoing_webhooks (tenant);

-- ---------------------------------------------------------------------------
-- BOTS: una identidad del tenant con token Bearer propio (prefijo bot_). Puede
-- publicar por la API en los canales a los que se le añada (reusa channel_members
-- con subject = bot:<id>). El token se guarda hasheado; solo se ve al crear.
-- ---------------------------------------------------------------------------
CREATE TABLE bots (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text        NOT NULL,
    name       text        NOT NULL,
    token_hash text        NOT NULL,
    owner_sub  text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT bots_name_not_blank CHECK (length(name) > 0)
);

CREATE UNIQUE INDEX bots_token_hash ON bots (token_hash);
CREATE INDEX bots_by_tenant ON bots (tenant);

-- ---------------------------------------------------------------------------
-- RLS: aislamiento por tenant + política PERMISSIVE de mantenimiento para el
-- lookup cross-tenant por token (entrante público / auth de bot).
-- ---------------------------------------------------------------------------
ALTER TABLE incoming_webhooks ENABLE ROW LEVEL SECURITY;
ALTER TABLE incoming_webhooks FORCE ROW LEVEL SECURITY;
ALTER TABLE outgoing_webhooks ENABLE ROW LEVEL SECURITY;
ALTER TABLE outgoing_webhooks FORCE ROW LEVEL SECURITY;
ALTER TABLE bots ENABLE ROW LEVEL SECURITY;
ALTER TABLE bots FORCE ROW LEVEL SECURITY;

CREATE POLICY incoming_webhooks_tenant_isolation ON incoming_webhooks
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY incoming_webhooks_maintenance ON incoming_webhooks
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY outgoing_webhooks_tenant_isolation ON outgoing_webhooks
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY bots_tenant_isolation ON bots
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY bots_maintenance ON bots
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- ---------------------------------------------------------------------------
-- Limitador token-bucket compartido entre réplicas (pg.RateLimiter): protege el
-- endpoint público POST /hooks/{token} (por token+IP). Copia idempotente del DDL
-- de plataforma (golang-migrate usa UNA tabla de versiones por BD, así que el FS
-- de plataforma no se aplica en paralelo). Igual que services/id (mig. 0005).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS platform_rate_limits (
    bucket_key text PRIMARY KEY,
    tokens     double precision NOT NULL,
    updated_at timestamptz      NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS platform_rate_limits_updated_at_idx
    ON platform_rate_limits (updated_at);
