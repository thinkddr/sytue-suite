-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Benchmarks anónimos: qué tamaño le basta a una app COMO LA TUYA.
--
-- El motor de rightsizing (0026) decide con el histórico del PROPIO proyecto, y
-- por eso no puede decir nada durante los primeros días —que es justo cuando el
-- cliente elige tamaño a ciegas y casi siempre se pasa—. Esta tabla cubre ese
-- hueco: el agregado de las apps comparables se puede enseñar el día cero.
--
-- INVARIANTE DE PRIVACIDAD, y es lo único que hay que entender de esta tabla:
-- agregar entre inquilinos es EXACTAMENTE lo que la RLS existe para impedir. Se
-- permite aquí, y solo aquí, bajo tres condiciones que no son negociables:
--
--  1. NO hay columna tenant, y no la puede haber: una fila no pertenece a nadie.
--     Cada fila resume >= 5 empresas distintas.
--  2. El umbral de k-anonimato se aplica AL ESCRIBIR, no al leer, y está aquí
--     como CHECK además de en el código (domain.MinCohortTenants). Duplicado a
--     propósito: ni un error de programación puede publicar la cohorte de dos
--     clientes, y bajar el umbral obliga a tocar las dos mitades.
--  3. Solo se publican percentiles del PORCENTAJE de uso sobre el límite del
--     tamaño, nunca cifras absolutas ni identificadores. Con 5 empresas, saber
--     que la mediana de la cohorte es el 12 % no permite deducir el consumo de
--     ninguna en concreto.
CREATE TABLE sizing_benchmarks (
    -- La clave: las tres cosas que de verdad predicen el consumo. Se corresponde
    -- 1:1 con domain.CohortKey.
    framework text NOT NULL,
    -- Tipos de servicio gestionado, ordenados y unidos por «+» (domain.ServicesKey).
    -- Cadena vacía = proyecto sin servicios.
    services  text NOT NULL,
    size      text NOT NULL,

    -- p50/p90 del p95 de memoria de cada proyecto, en % del límite de SU tamaño.
    -- El p90 es el que impide recomendar un tamaño donde una de cada diez apps de
    -- la cohorte no cabría.
    p50_memory_percent integer NOT NULL CHECK (p50_memory_percent BETWEEN 0 AND 100),
    p90_memory_percent integer NOT NULL CHECK (p90_memory_percent BETWEEN 0 AND 100),

    -- Auditoría de la publicación: sin estas dos columnas nadie puede comprobar a
    -- posteriori que el umbral se respetó.
    tenants  integer NOT NULL CHECK (tenants >= 5),
    projects integer NOT NULL CHECK (projects >= tenants),

    computed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (framework, services, size)
);

ALTER TABLE sizing_benchmarks ENABLE ROW LEVEL SECURITY;
ALTER TABLE sizing_benchmarks FORCE ROW LEVEL SECURITY;

-- Lectura abierta A PROPÓSITO: el contenido ya es anónimo por construcción (ver
-- el CHECK de arriba), así que no hay nada que segmentar por inquilino. Se declara
-- explícitamente en vez de dejar la tabla sin RLS para que quede escrito que es
-- una decisión y no un descuido.
CREATE POLICY sizing_benchmarks_read ON sizing_benchmarks
    FOR SELECT USING (true);

-- Escritura SOLO en mantenimiento: el cálculo recorre todos los inquilinos en un
-- proceso, y ninguna petición de usuario tiene por qué tocar esta tabla. Mismo
-- criterio exacto que la 0026 (`app.maintenance='on'`, que solo fija
-- pg.InMaintenanceTx).
CREATE POLICY sizing_benchmarks_maintenance ON sizing_benchmarks
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
