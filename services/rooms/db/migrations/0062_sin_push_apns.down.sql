-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelve a crear la tabla de 0032, vacía.

CREATE TABLE push_apns (
    tenant     text        NOT NULL,
    subject    text        NOT NULL,
    token      text        NOT NULL,   -- token de dispositivo APNs (hex)
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, token)
);

CREATE INDEX push_apns_subject_idx ON push_apns (tenant, subject);

ALTER TABLE push_apns ENABLE ROW LEVEL SECURITY;
ALTER TABLE push_apns FORCE ROW LEVEL SECURITY;

CREATE POLICY push_apns_tenant_isolation ON push_apns
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY push_apns_platform_merge ON push_apns
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
