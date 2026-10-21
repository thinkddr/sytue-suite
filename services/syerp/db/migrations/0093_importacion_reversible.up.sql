-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- IMPORTACIÓN REVERSIBLE.
--
-- `POST /{modelo}/importar` escribía DIRECTAMENTE: cinco mil filas de un CSV
-- ajeno entraban en la base sin simulación, sin lote y sin marcha atrás. Es el
-- gesto más destructivo del ERP —lo hace quien acaba de llegar de otro sistema,
-- con un fichero que nadie ha revisado— y era el único sin red.
--
-- Deshacerlo a mano no es una alternativa: las filas creadas no se distinguen
-- de las de siempre, y de las actualizadas ya no queda lo que había antes.
--
-- Dos tablas:
--
--   `importacion`       el lote: qué modelo, qué fichero, cuándo, quién, cuánto.
--   `importacion_fila`  qué le pasó a CADA registro, con lo que había ANTES.
--
-- `datos_antes` es lo que hace posible la vuelta atrás de una ACTUALIZACIÓN;
-- sin ella solo se podrían borrar las creadas, que es media reversión y la
-- mitad menos útil.
--
-- `updated_at_tras` es el `updated_at` del registro justo después de que la
-- importación lo escribiera. Al revertir se compara: si no coincide, alguien
-- tocó ese registro DESPUÉS y su trabajo no se pisa — se cuenta aparte y se
-- dice. Una reversión que borra el trabajo de otro es peor que no revertir.

CREATE TABLE importacion (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    modelo       text        NOT NULL,
    -- Nombre del fichero, si el cliente lo mandó: es como lo reconoce quien lo subió.
    fichero      text        NOT NULL DEFAULT '',
    filas        integer     NOT NULL DEFAULT 0,
    creados      integer     NOT NULL DEFAULT 0,
    actualizados integer     NOT NULL DEFAULT 0,
    fallos       integer     NOT NULL DEFAULT 0,
    por          text        NOT NULL DEFAULT '',
    creada_en    timestamptz NOT NULL DEFAULT now(),
    -- Cuándo se deshizo, y quién. Una importación revertida no se revierte otra vez.
    revertida_en  timestamptz,
    revertida_por text       NOT NULL DEFAULT '',

    CONSTRAINT importacion_fichero CHECK (length(fichero) <= 300)
);

CREATE INDEX importacion_recientes ON importacion (tenant, creada_en DESC);

CREATE TABLE importacion_fila (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    importacion_id  uuid        NOT NULL REFERENCES importacion(id) ON DELETE CASCADE,
    registro_id     uuid        NOT NULL,
    accion          text        NOT NULL,
    -- Solo para 'actualizado': lo que tenía antes. Los tres, y no solo `data`:
    -- el nombre legible y el estado viven en sus propias columnas, así que una
    -- reversión que solo devolviera `data` dejaría el registro con el nombre
    -- que le puso la importación.
    datos_antes     jsonb,
    etiqueta_antes  text        NOT NULL DEFAULT '',
    estado_antes    text        NOT NULL DEFAULT '',
    updated_at_tras timestamptz NOT NULL,

    CONSTRAINT importacion_fila_accion CHECK (accion IN ('creado', 'actualizado'))
);

CREATE INDEX importacion_fila_del_lote ON importacion_fila (tenant, importacion_id);

ALTER TABLE importacion ENABLE ROW LEVEL SECURITY;
ALTER TABLE importacion FORCE ROW LEVEL SECURITY;
ALTER TABLE importacion_fila ENABLE ROW LEVEL SECURITY;
ALTER TABLE importacion_fila FORCE ROW LEVEL SECURITY;

CREATE POLICY importacion_tenant_isolation ON importacion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY importacion_fila_tenant_isolation ON importacion_fila
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Literales, como en la 0009 y la 0011: el gate de fusión lee el SQL sin ejecutarlo.
CREATE POLICY importacion_platform_merge ON importacion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY importacion_fila_platform_merge ON importacion_fila
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
