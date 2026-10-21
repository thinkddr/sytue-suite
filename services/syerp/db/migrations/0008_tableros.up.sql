-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Los tableros de Sytue AI. Se guarda el INFORME (qué fuentes, filtros y medidas)
-- y no sus datos: al abrir un tablero se vuelve a ejecutar, con los permisos de
-- quien lo mira.

CREATE TABLE tableros (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text        NOT NULL,
    titulo     text        NOT NULL,
    informe    jsonb       NOT NULL,
    creado_por text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT tableros_informe_es_objeto CHECK (jsonb_typeof(informe) = 'object')
);

CREATE INDEX tableros_recientes ON tableros (tenant, created_at DESC);

ALTER TABLE tableros ENABLE ROW LEVEL SECURITY;
ALTER TABLE tableros FORCE ROW LEVEL SECURITY;

CREATE POLICY tableros_tenant_isolation ON tableros
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY tableros_platform_merge ON tableros
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
