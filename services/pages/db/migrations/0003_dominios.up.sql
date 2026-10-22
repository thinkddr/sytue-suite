-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Pages, Dominios y Abuso (PAGES-ENCARGO §8.3, §8.5): dominios propios con verificación
-- DNS/TXT, estado de activación y denuncias de abuso.

CREATE TABLE domains (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant         text NOT NULL,
    site_id        uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    host           text NOT NULL CHECK (length(host) <= 253 AND host ~ '^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$'),
    parent_id      uuid REFERENCES domains(id) ON DELETE CASCADE,
    token          text NOT NULL CHECK (token ~ '^[0-9a-f]{32}$'),
    status         text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'active', 'failed')),
    detail         text NOT NULL DEFAULT '',
    checked_at     timestamptz,
    verified_at    timestamptz,
    activated_at   timestamptz,
    revalidated_at timestamptz,
    created_by     text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (site_id, host)
);

CREATE UNIQUE INDEX domains_host_en_uso ON domains (host) WHERE status IN ('verified', 'active');
CREATE INDEX domains_por_tenant ON domains (tenant, created_at DESC);
CREATE INDEX domains_por_sitio ON domains (site_id, status);

-- RLS para domains con las tres políticas obligatorias y política pública condicional
ALTER TABLE domains ENABLE ROW LEVEL SECURITY;
ALTER TABLE domains FORCE ROW LEVEL SECURITY;

CREATE POLICY domains_tenant_isolation ON domains
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY domains_platform_merge ON domains
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY domains_mantenimiento ON domains
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY domains_publico ON domains FOR SELECT
    USING (current_setting('app.pages_publico', true) = 'on' AND status IN ('verified', 'active'));

-- Denuncias de abuso: dato del OPERADOR (sin tenant ni RLS, solo lectura en mantenimiento)
CREATE TABLE abuse_reports (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id        uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    host           text NOT NULL,
    path           text NOT NULL DEFAULT '' CHECK (length(path) <= 500),
    reason         text NOT NULL CHECK (reason IN ('phishing', 'malware', 'illegal', 'copyright', 'other')),
    details        text NOT NULL DEFAULT '' CHECK (length(details) <= 4000),
    reporter_email text NOT NULL DEFAULT '' CHECK (length(reporter_email) <= 254),
    status         text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'actioned', 'dismissed')),
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX abuse_reports_por_sitio ON abuse_reports (site_id, created_at DESC);
CREATE INDEX abuse_reports_abiertas ON abuse_reports (status, created_at) WHERE status = 'open';
