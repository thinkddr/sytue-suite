-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Ola B — seguridad/authz de la API: credenciales programáticas, RBAC por
-- membership, secreto de webhook por proyecto y estado del rate-limiter.
--
-- 1) api_tokens: credenciales de CI/CLI (Bearer <token>). Solo se persiste el
--    HASH SHA-256 del token; el secreto se muestra UNA vez al crear. Scopes
--    read|write|deploy. RLS deny-by-default por tenant (patrón 0001).
-- 2) memberships: colaboradores de la organización con rol efectivo
--    (owner|admin|member|billing). PK (tenant, sub). RLS por tenant.
-- 3) projects.webhook_secret_enc: secreto HMAC del webhook git POR PROYECTO,
--    cifrado en reposo (AES-256-GCM, internal/crypto). Cierra la falsificación
--    cross-tenant que permitía el secreto global único.
-- 4) platform_rate_limits: estado del token-bucket compartido (pg.RateLimiter).
--    Copia literal de platform/pkg/pg/migrations/0001_platform_rate_limits.up.sql
--    (golang-migrate usa UNA tabla de versiones por BD; el DDL es IF NOT EXISTS).

-- 1) Tokens de API ------------------------------------------------------------
CREATE TABLE api_tokens (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    name         text        NOT NULL,
    -- HASH SHA-256 (hex) del token en claro; el secreto NUNCA se persiste.
    token_hash   text        NOT NULL,
    scope        text        NOT NULL DEFAULT 'read' CHECK (scope IN ('read', 'write', 'deploy')),
    created_by   text        NOT NULL DEFAULT '',
    expires_at   timestamptz,
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- El hash es un secreto aleatorio de 256 bits: único global (la resolución del
-- token en el middleware es cross-tenant, por hash, antes de conocer el tenant).
CREATE UNIQUE INDEX api_tokens_hash_idx ON api_tokens (token_hash);
-- Listado por tenant (RLS) ordenado por recencia.
CREATE INDEX api_tokens_tenant_idx ON api_tokens (tenant, created_at DESC);

-- 2) Memberships --------------------------------------------------------------
CREATE TABLE memberships (
    tenant     text        NOT NULL,
    sub        text        NOT NULL,
    role       text        NOT NULL DEFAULT 'member'
               CHECK (role IN ('owner', 'admin', 'member', 'billing')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, sub)
);
-- El listado de miembros por tenant usa el prefijo de la PK (tenant, ...); el
-- conjunto por organización está acotado, así que no requiere índice extra.

-- 3) Secreto de webhook por proyecto -----------------------------------------
ALTER TABLE projects ADD COLUMN webhook_secret_enc bytea;

-- 4) Estado del rate-limiter (copia literal de la migración de plataforma) ----
CREATE TABLE IF NOT EXISTS platform_rate_limits (
    bucket_key text PRIMARY KEY,
    tokens     double precision NOT NULL,
    updated_at timestamptz      NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS platform_rate_limits_updated_at_idx
    ON platform_rate_limits (updated_at);

-- RLS deny-by-default por tenant (patrón 0001) --------------------------------
ALTER TABLE api_tokens  ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_tokens  FORCE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;

CREATE POLICY api_tokens_tenant_isolation ON api_tokens
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY memberships_tenant_isolation ON memberships
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Resolución del token SIN tenant en el contexto: el middleware de auth resuelve
-- tenant+scope a partir del hash del Bearer ANTES de conocer el tenant. Se usa el
-- mismo mecanismo cross-tenant que el reconciliador (0003): una política PERMISSIVE
-- de mantenimiento que solo abre cuando la sesión fija app.maintenance='on'
-- (pg.InMaintenanceTx). Las transacciones de negocio nunca lo fijan, así que el
-- aislamiento por tenant queda intacto. Se prefiere a una función SECURITY DEFINER
-- porque, con FORCE RLS y un dueño de tabla SIN BYPASSRLS (el rol de la app posee
-- su BD), el definer también queda sujeto a RLS y no vería ninguna fila.
CREATE POLICY api_tokens_maintenance ON api_tokens
    USING (current_setting('app.maintenance', true) = 'on');
