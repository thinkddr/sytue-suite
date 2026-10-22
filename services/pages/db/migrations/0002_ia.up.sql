-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Pages, IA (PAGES-ENCARGO §5, §9.2-9.3): claves BYOK selladas con keycrypto
-- y trabajos asíncronos de diseño, traducción y exportación.

CREATE TABLE ai_keys (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant             text NOT NULL,
    provider           text NOT NULL CHECK (provider IN ('openai', 'anthropic', 'zai', 'mistral')),
    model              text NOT NULL CHECK (length(model) BETWEEN 1 AND 100),
    key_enc            text NOT NULL CHECK (key_enc LIKE 'encv1:%'),
    notice_accepted_at timestamptz,
    accepted_by        text NOT NULL,
    CHECK (provider = 'mistral' OR notice_accepted_at IS NOT NULL),
    checked_at         timestamptz NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_keys_por_tenant ON ai_keys (tenant, created_at DESC);

CREATE TABLE ai_jobs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text NOT NULL,
    site_id     uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind IN ('design', 'translate', 'export')),
    status      text NOT NULL CHECK (status IN ('running', 'done', 'failed')),
    input       jsonb NOT NULL,
    result      jsonb,
    result_key  text NOT NULL DEFAULT '',
    error       text NOT NULL DEFAULT '',
    provider    text NOT NULL,
    byok        boolean NOT NULL,
    model       text NOT NULL DEFAULT '',
    tokens_in   integer NOT NULL DEFAULT 0,
    tokens_out  integer NOT NULL DEFAULT 0,
    created_by  text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE INDEX ai_jobs_cupo ON ai_jobs (tenant, created_at) WHERE NOT byok;
CREATE INDEX ai_jobs_por_tenant ON ai_jobs (tenant, created_at DESC);
CREATE INDEX ai_jobs_running ON ai_jobs (status, created_at) WHERE status = 'running';

-- RLS con las tres políticas obligatorias de la plataforma
ALTER TABLE ai_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY ai_keys_tenant_isolation ON ai_keys
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY ai_keys_platform_merge ON ai_keys
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY ai_keys_mantenimiento ON ai_keys
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

ALTER TABLE ai_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY ai_jobs_tenant_isolation ON ai_jobs
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY ai_jobs_platform_merge ON ai_jobs
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE POLICY ai_jobs_mantenimiento ON ai_jobs
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
