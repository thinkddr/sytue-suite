-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- SEGREGACIÓN DE FUNCIONES: lo que alguien puede HACER en un módulo, y no solo
-- con cuánto alcance.
--
-- Hasta aquí el ERP solo tenía cuatro niveles por módulo (0009), y un nivel es
-- una escalera: quien llegaba a «usuario» para crear una factura llegaba también
-- para asentarla. Eso es justo lo que la separación de funciones prohíbe —crear,
-- aprobar, pagar, conciliar, auditar y administrar no se implican entre sí—, y no
-- se puede expresar con un número ordenado.
--
-- CÓMO CONVIVE CON LOS NIVELES. Sin fila aquí, los verbos se DEDUCEN del nivel
-- (permisos.VerbosPorDefecto), así que ninguna organización cambia de
-- comportamiento al desplegar esto. Con fila, la lista de esta tabla es la
-- COMPLETA para esa persona en ese módulo y sustituye a la deducida: es el gesto
-- explícito con el que una organización separa funciones.
--
-- Y vincula TAMBIÉN a owner/admin de la organización. Es deliberado: una
-- separación de funciones de la que el administrador se escapa por su rol no
-- separa nada. Sin fila siguen siendo administradores de todo, como hasta ahora.

CREATE TABLE permisos_verbo (
    tenant          text        NOT NULL,
    sub             text        NOT NULL,
    modulo          text        NOT NULL,
    -- La lista COMPLETA de verbos de esta persona en este módulo. Vacía = no
    -- puede ningún gesto sensible, que es distinto de no tener fila.
    verbos          text[]      NOT NULL DEFAULT '{}',
    actualizado_por text        NOT NULL,
    actualizado_en  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant, sub, modulo),
    -- El catálogo de verbos vive en Go (permisos.Verbos) y aquí como CHECK: una
    -- fila con un verbo inventado concedería algo que nadie comprueba.
    CONSTRAINT permisos_verbo_conocidos CHECK (
        verbos <@ ARRAY['crear','aprobar','asentar','pagar','conciliar','auditar','administrar']::text[]
    )
);

ALTER TABLE permisos_verbo ENABLE ROW LEVEL SECURITY;
ALTER TABLE permisos_verbo FORCE ROW LEVEL SECURITY;

CREATE POLICY permisos_verbo_tenant_isolation ON permisos_verbo
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Literal, como en la 0009 y la 0011: el gate de fusión lee el SQL sin ejecutarlo.
CREATE POLICY permisos_verbo_platform_merge ON permisos_verbo
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
