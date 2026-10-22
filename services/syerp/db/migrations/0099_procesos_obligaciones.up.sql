-- SPDX-License-Identifier: Apache-2.0
-- Progreso asistido, compartido por la organización. Archivar un justificante
-- no certifica la aceptación de una declaración por la administración.
CREATE TABLE procesos_obligaciones (
    tenant text NOT NULL,
    clave text NOT NULL,
    periodo text NOT NULL,
    fase smallint NOT NULL DEFAULT 0 CHECK (fase BETWEEN 0 AND 4),
    referencia text NOT NULL DEFAULT '',
    documentos text[] NOT NULL DEFAULT '{}',
    actualizado_por text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, clave, periodo)
);
ALTER TABLE procesos_obligaciones ENABLE ROW LEVEL SECURITY;
ALTER TABLE procesos_obligaciones FORCE ROW LEVEL SECURITY;
CREATE POLICY procesos_obligaciones_tenant_isolation ON procesos_obligaciones
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY procesos_obligaciones_platform_merge ON procesos_obligaciones
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- La identidad idempotente del TPV también tiene un cerrojo en PostgreSQL.
CREATE UNIQUE INDEX records_tpv_operacion ON records (tenant, model, (data->>'tpv_operacion'))
    WHERE model IN ('factura_venta', 'presupuesto') AND data->>'tpv_operacion' IS NOT NULL;
