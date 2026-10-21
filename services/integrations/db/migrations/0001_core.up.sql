-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base del Sytue Integration Gateway (OLA 1): el núcleo de la pasarela de
-- integraciones a terceros.
--   - app_templates   : catálogo GLOBAL de plantillas de conector instalables.
--   - app_instances   : instancias configuradas por organización (RLS por org).
--   - instance_secrets: secretos sellados (AES-256-GCM "encv1"); NUNCA en claro.
--   - egress_audit    : registro de tráfico saliente a terceros (solo metadatos).
--   - events_outbox   : bus transaccional (platform/pkg/events); para W2+.
--   - platform_rate_limits: estado del token-bucket compartido (pg.RateLimiter).
--
-- Multi-tenant: cada tabla por-org lleva `org_id` (slug de organización). RLS
-- deny-by-default aísla por la variable de sesión `app.tenant` (la fija
-- pg.SetTenant en cada transacción). El VALOR de los secretos jamás se lee salvo
-- para descifrarlo en memoria al fabricar el Runtime de un conector.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- app_templates — catálogo global de plantillas de conector. Sin RLS: es un
-- catálogo público a cualquier usuario autenticado. Manifest describe la
-- config/secretos/capacidades esperadas por el driver (JSON opaco al núcleo).
-- ---------------------------------------------------------------------------
CREATE TABLE app_templates (
    key        text        PRIMARY KEY,
    manifest   jsonb       NOT NULL DEFAULT '{}'::jsonb,
    tier       text        NOT NULL DEFAULT 'standard',
    enabled    boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- app_instances — instancia de una plantilla para una organización.
-- config es PÚBLICA (sin secretos). level_enabled: capacidades habilitadas.
-- ---------------------------------------------------------------------------
CREATE TABLE app_instances (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        text        NOT NULL,
    template_key  text        NOT NULL REFERENCES app_templates (key),
    name          text        NOT NULL,
    config        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    status        text        NOT NULL DEFAULT 'draft'
                              CHECK (status IN ('draft', 'active', 'error', 'disabled')),
    level_enabled text[]      NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT app_instances_name_not_blank CHECK (length(name) > 0)
);

-- Listado por org con paginación keyset (created_at, id) DESC; sin N+1.
CREATE INDEX app_instances_org_feed ON app_instances (org_id, created_at DESC, id DESC);
-- Búsqueda por plantilla dentro de la org.
CREATE INDEX app_instances_org_template ON app_instances (org_id, template_key);

-- ---------------------------------------------------------------------------
-- instance_secrets — secretos sellados (encv1: base64) por instancia. El valor
-- vive SIEMPRE cifrado; solo se abre en memoria al ejecutar. ON DELETE CASCADE:
-- borrar la instancia borra sus secretos.
-- ---------------------------------------------------------------------------
CREATE TABLE instance_secrets (
    instance_id uuid        NOT NULL REFERENCES app_instances (id) ON DELETE CASCADE,
    org_id      text        NOT NULL,
    name        text        NOT NULL,
    value_enc   text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (instance_id, name)
);

-- ---------------------------------------------------------------------------
-- egress_audit — una fila por petición saliente a un tercero. SOLO metadatos:
-- host/método/status/bytes/latencia. `auth` guarda 'REDACTED' si hubo cabecera
-- Authorization; NUNCA el token. Sin payloads.
-- ---------------------------------------------------------------------------
CREATE TABLE egress_audit (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id      text        NOT NULL,
    instance_id uuid,
    host        text        NOT NULL DEFAULT '',
    method      text        NOT NULL DEFAULT '',
    status      int         NOT NULL DEFAULT 0,
    req_bytes   bigint      NOT NULL DEFAULT 0,
    resp_bytes  bigint      NOT NULL DEFAULT 0,
    latency_ms  bigint      NOT NULL DEFAULT 0,
    auth        text        NOT NULL DEFAULT '',
    err         text        NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL DEFAULT now()
);

-- Consulta del registro por org, orden temporal descendente (auditoría).
CREATE INDEX egress_audit_org_occurred ON egress_audit (org_id, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- events_outbox — bus transaccional compartido (platform/pkg/events). DDL pegado
-- literal de events.OutboxTableDDL (golang-migrate usa UNA tabla de versiones por
-- BD; el FS de plataforma no puede aplicarse en paralelo). Para publicaciones de
-- W2+ (p. ej. "instancia verificada"); sin uso en OLA 1.
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
    last_error   text
);
CREATE INDEX IF NOT EXISTS events_outbox_unprocessed_idx
    ON events_outbox (created_at)
    WHERE processed_at IS NULL;

-- ---------------------------------------------------------------------------
-- platform_rate_limits — estado del token-bucket compartido entre réplicas
-- (pg.RateLimiter). Copia literal de la migración de plataforma; DDL idempotente.
-- Lo usa el limitador de egress por org:instance. Sin RLS (estado global).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS platform_rate_limits (
    bucket_key text PRIMARY KEY,
    tokens     double precision NOT NULL,
    updated_at timestamptz      NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS platform_rate_limits_updated_at_idx
    ON platform_rate_limits (updated_at);

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por org deny-by-default (CONVENTIONS §10).
-- Las políticas comparan `org_id` con current_setting('app.tenant', true), que
-- fija pg.SetTenant LOCAL a cada transacción. app_templates queda fuera (global);
-- events_outbox/platform_rate_limits son estado de servicio (no por-org).
-- ---------------------------------------------------------------------------
ALTER TABLE app_instances    ENABLE ROW LEVEL SECURITY;
ALTER TABLE instance_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE egress_audit     ENABLE ROW LEVEL SECURITY;

ALTER TABLE app_instances    FORCE ROW LEVEL SECURITY;
ALTER TABLE instance_secrets FORCE ROW LEVEL SECURITY;
ALTER TABLE egress_audit     FORCE ROW LEVEL SECURITY;

CREATE POLICY app_instances_org_isolation ON app_instances
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));
CREATE POLICY instance_secrets_org_isolation ON instance_secrets
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));
CREATE POLICY egress_audit_org_isolation ON egress_audit
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));

-- ---------------------------------------------------------------------------
-- Rol de aplicación (deny-by-default reforzado por RLS): las políticas SOLO se
-- ejercen sobre roles NO superusuario y SIN BYPASSRLS. El store hace
-- `SET LOCAL ROLE integrations_app` en cada transacción por-org, de modo que el
-- aislamiento es INDEPENDIENTE de cómo se conecte el pool (superusuario en dev/CI).
-- En producción el despliegue hace `GRANT integrations_app TO <rol_de_la_app>`.
-- ---------------------------------------------------------------------------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'integrations_app') THEN
        CREATE ROLE integrations_app NOLOGIN NOINHERIT NOBYPASSRLS;
    END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO integrations_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO integrations_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO integrations_app;

-- Membresía para el dueño de las migraciones (dev/CI). Idempotente.
DO $$
BEGIN
    EXECUTE format('GRANT integrations_app TO %I', current_user);
END
$$;
