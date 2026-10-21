-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Cuota COMPARTIDA con otras apps de la suite (CAST-ENCARGO §8, 2026-09-27): la
-- del plan es una sola —Free 5 GB · Pro 200 GB · Enterprise 1 TB— y Drive es su
-- dueño. Sytue Cast guarda los vídeos en su propio bucket, pero lo que ocupan
-- cuenta aquí: cada app declara, con el secreto interno, el TOTAL que ocupa en
-- cada organización (`PUT /internal/quota/external/{app}`), y `used_bytes` de la
-- cuota que ve la persona pasa a ser `quotas.used_bytes` + la suma de esta tabla.
--
-- Una fila por (organización, app) con el valor ABSOLUTO, no un contador de
-- deltas: la declaración es idempotente y una llamada perdida no deja la cuota
-- descuadrada para siempre, la siguiente la corrige. `quotas.used_bytes` sigue
-- siendo solo lo de Drive; así el medidor puede desglosar quién ocupa qué.
CREATE TABLE quota_external_usage (
    tenant     text        NOT NULL,
    app        text        NOT NULL,
    bytes      bigint      NOT NULL DEFAULT 0 CHECK (bytes >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, app)
);

ALTER TABLE quota_external_usage ENABLE ROW LEVEL SECURITY;
ALTER TABLE quota_external_usage FORCE ROW LEVEL SECURITY;

CREATE POLICY quota_external_usage_tenant_isolation ON quota_external_usage
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Fusión de organizaciones (0015 solo cubrió las tablas que ya existían).
CREATE POLICY quota_external_usage_platform_merge ON quota_external_usage
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
