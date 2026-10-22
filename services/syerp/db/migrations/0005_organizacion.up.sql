-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El perfil de la organización: lo que el asistente de arranque recoge y de lo
-- que sale su calendario de obligaciones. UNA fila por organización.
--
-- Las condiciones (IVA, empleados, intracomunitario…) van en jsonb y no en
-- columnas: son los interruptores que declara `obligaciones.Condiciones`, y
-- añadir uno debe ser tocar el dato, no una migración. El país y la forma sí
-- son columnas: son claves ajenas a los ficheros de país y se consultan siempre.

CREATE TABLE organizacion (
    tenant       text        PRIMARY KEY,
    nombre       text        NOT NULL DEFAULT '',
    nif          text        NOT NULL DEFAULT '',
    direccion    text        NOT NULL DEFAULT '',
    cp           text        NOT NULL DEFAULT '',
    ciudad       text        NOT NULL DEFAULT '',
    pais         char(2)     NOT NULL,
    forma        text        NOT NULL,
    condiciones  jsonb       NOT NULL DEFAULT '{}'::jsonb,
    actualizado_por text     NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT organizacion_condiciones_es_objeto CHECK (jsonb_typeof(condiciones) = 'object')
);

ALTER TABLE organizacion ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizacion FORCE ROW LEVEL SECURITY;

CREATE POLICY organizacion_tenant_isolation ON organizacion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Odoo entra como tercer proveedor de conexión: importa por la misma pasarela
-- que Salesforce (el driver ya existe en el Integration Gateway).
ALTER TABLE conexiones DROP CONSTRAINT conexiones_proveedor_check;
ALTER TABLE conexiones ADD CONSTRAINT conexiones_proveedor_check
    CHECK (proveedor IN ('stripe', 'salesforce', 'odoo'));
