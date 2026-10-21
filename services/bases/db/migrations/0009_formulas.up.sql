-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Campos de FÓRMULA (ADR 0068, bases-2, pieza 3).
--
-- Un campo de fórmula guarda su expresión y el tipo de su resultado en `options`
-- ({"formula": "…", "result": "number"|"text"}) y su valor, ya calculado por el servicio,
-- en `base_records.data` como cualquier otro: por eso filtros, orden, exportación y
-- conector no cambian.
ALTER TABLE base_fields DROP CONSTRAINT base_fields_kind_check;
ALTER TABLE base_fields ADD CONSTRAINT base_fields_kind_check
    CHECK (kind IN ('text', 'number', 'date', 'checkbox', 'select', 'multiselect', 'link', 'formula'));

-- El esquema entero de una base (para validar una fórmula y saber qué depende de qué).
CREATE INDEX base_fields_por_base ON base_fields (tenant, base_id);

-- Recálculos PENDIENTES. Cambiar un registro del que dependen pocos se recalcula en la
-- misma transacción; cuando son muchos (o al crear una fórmula en una tabla grande), se
-- anota aquí la tabla entera y un bucle del servicio la recorre por tramos. Una fila por
-- tabla: pedirlo otra vez mientras está en curso lo reinicia (`desde_*` a NULL).
CREATE TABLE base_recalc (
    tenant       text NOT NULL,
    table_id     uuid NOT NULL,
    base_id      uuid NOT NULL,
    -- Hasta dónde llegó, en el orden (created_at, id) del índice de registros.
    desde_t      timestamptz,
    desde_id     uuid,
    requested_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, table_id),
    FOREIGN KEY (tenant, table_id, base_id) REFERENCES base_tables (tenant, id, base_id)
        ON DELETE CASCADE ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX base_recalc_por_antiguedad ON base_recalc (requested_at);

ALTER TABLE base_recalc ENABLE ROW LEVEL SECURITY;
ALTER TABLE base_recalc FORCE ROW LEVEL SECURITY;
CREATE POLICY base_recalc_tenant_isolation ON base_recalc
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY base_recalc_platform_merge ON base_recalc
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
-- Solo lectura, para que el bucle encuentre trabajo sin saber de qué organización es; el
-- trabajo en sí va en la transacción de ESA organización.
CREATE POLICY base_recalc_publico ON base_recalc FOR SELECT
    USING (current_setting('app.bases_publico', true) = 'on');
