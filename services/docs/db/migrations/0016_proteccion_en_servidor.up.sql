-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PROTECCIÓN DE HOJA Y LIBRO CUMPLIDA EN EL SERVIDOR (ROADMAP-OFFICE permisos-1).
--
-- Hasta ahora solo la aplicaba el editor, y la contraseña viajaba EN CLARO dentro del
-- modelo (la leía cualquier lector con GET /state). Decisión del dueño (2026-10-01):
-- la contraseña se guarda solo como hash con sal, y el servidor decide.
--
-- Dos tablas, ambas con una fila por documento de hoja de cálculo:
--   · doc_proteccion: el último modelo doc:sheet EFECTIVO si hay alguna protección
--     (así validar una escritura es una lectura y no un replay del historial), más los
--     metadatos LWW del registro, sin los cuales no se sabe si una operación gana o el
--     registro la descarta. La fila existe una vez evaluado el documento (la primera
--     escritura que toca el modelo la crea, y es también cuando se migran las
--     contraseñas en claro de antes).
--   · doc_proteccion_claves: el hash por ámbito («libro» o «hoja:<nombre>») con el
--     esquema de OOXML (algorithmName, saltValue, spinCount, hashValue) para que viaje
--     tal cual en un XLSX. Nunca se sirve a ningún cliente.
CREATE TABLE doc_proteccion (
    document_id uuid        PRIMARY KEY REFERENCES documents (id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    modelo      text        NOT NULL DEFAULT '',
    hay_registro boolean    NOT NULL DEFAULT false,
    lamport     bigint      NOT NULL DEFAULT 0,
    dot_actor   text        NOT NULL DEFAULT '',
    dot_counter bigint      NOT NULL DEFAULT 0,
    actualizada timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE doc_proteccion_claves (
    document_id uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    ambito      text        NOT NULL,
    algoritmo   text        NOT NULL,
    sal         bytea       NOT NULL,
    vueltas     integer     NOT NULL CHECK (vueltas >= 0),
    hash        bytea       NOT NULL,
    creada      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (document_id, ambito)
);

-- Aislamiento por organización (deny-by-default), el mismo patrón que 0001 y 0008.
ALTER TABLE doc_proteccion ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_proteccion FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_proteccion_tenant_isolation ON doc_proteccion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_proteccion_platform_merge ON doc_proteccion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

ALTER TABLE doc_proteccion_claves ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_proteccion_claves FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_proteccion_claves_tenant_isolation ON doc_proteccion_claves
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_proteccion_claves_platform_merge ON doc_proteccion_claves
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

GRANT SELECT, INSERT, UPDATE, DELETE ON doc_proteccion, doc_proteccion_claves TO docs_app;
