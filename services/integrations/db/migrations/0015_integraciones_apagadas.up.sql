-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- INTEGRACIONES DE SERIE APAGADAS POR LA ORGANIZACIÓN (decidido por el dueño el
-- 2026-10-19). Cada persona conecta sus nubes, chats y videollamadas desde la app con
-- solo el consentimiento; Admin solo las apaga, por proveedor. Encendidas por defecto:
-- la tabla guarda únicamente las que se apagaron.
CREATE TABLE disabled_integrations (
    org_id     text        NOT NULL,
    provider   text        NOT NULL CHECK (provider ~ '^[a-z0-9-]{2,40}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, provider)
);

ALTER TABLE disabled_integrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE disabled_integrations FORCE ROW LEVEL SECURITY;

CREATE POLICY disabled_integrations_org_isolation ON disabled_integrations
    USING (org_id = current_setting('app.tenant', true))
    WITH CHECK (org_id = current_setting('app.tenant', true));

GRANT SELECT, INSERT, DELETE ON disabled_integrations TO integrations_app;
