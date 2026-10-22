-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- AJUSTES DE LA ORGANIZACIÓN EN SYTUE PHOTOS: excluir del lote de IA.
--
-- El lote de vectores (`vectores-sacar`, `vectores-meter` y `vectores-serverless`)
-- recorre todas las organizaciones en una transacción de mantenimiento para
-- describir las fotos sin vector y calcular sus embeddings. Un `owner` o `admin`
-- puede sacar a su organización de ese lote automático.
--
-- Tres políticas RLS:
--   1. `org_settings_tenant_isolation`: cada organización lee y escribe solo su fila.
--   2. `org_settings_mantenimiento`: el lote corre bajo `pg.InMaintenanceTx`
--      (`app.maintenance = 'on'`, sin `app.tenant`); sin permiso de lectura en
--      mantenimiento, el `NOT EXISTS` sobre `org_settings` no vería ninguna fila
--      y seguiría incluyendo a las organizaciones excluidas en silencio.
--   3. `org_settings_platform_merge`: exigida por `platform/pkg/migcheck` en toda
--      tabla con columna `tenant`.

CREATE TABLE org_settings (
    tenant           text        PRIMARY KEY,
    exclude_ai_batch boolean     NOT NULL DEFAULT false,
    updated_by       text        NOT NULL DEFAULT '',
    updated_at       timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE org_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_settings FORCE ROW LEVEL SECURITY;

CREATE POLICY org_settings_tenant_isolation ON org_settings
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY org_settings_mantenimiento ON org_settings
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');

CREATE POLICY org_settings_platform_merge ON org_settings
    USING (current_setting('app.platform_merge', true) = 'on')
    WITH CHECK (current_setting('app.platform_merge', true) = 'on');
