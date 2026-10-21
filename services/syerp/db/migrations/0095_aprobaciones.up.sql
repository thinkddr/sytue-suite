-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- APROBACIONES CON UMBRAL Y CUATRO OJOS.
--
-- Los verbos (0092) separan QUIÉN puede hacer qué. Falta lo otro: que a partir
-- de cierto importe haga falta una SEGUNDA PERSONA, aunque quien lo hace tenga
-- el verbo. Sin esto, una sola persona con `asentar` metía una factura de
-- cincuenta mil euros en el libro sin que nadie más la viera — y ese es el
-- fraude interno típico de una pyme, no un escenario de libro.
--
-- EL DINERO VA EN CÉNTIMOS ENTEROS, y con su divisa al lado. Un umbral es un
-- importe, así que lleva las dos columnas: comparar «50000» de una tabla con
-- «50000» de un documento en otra moneda no es una comparación, es un error
-- esperando a ocurrir. Cuando las divisas no coinciden, el ERP NO convierte
-- —no hay fuente de tipos de cambio decidida— y pide aprobación igual: fallar
-- del lado seguro.
--
-- CUATRO OJOS: quien aprueba no puede ser quien creó el documento ni quien pidió
-- la aprobación. Eso NO se puede expresar en un CHECK (hace falta mirar el
-- registro), así que lo impone el store; aquí se guardan los tres subs para que
-- la comprobación tenga contra qué hacerse y para que se vea después.

-- Umbral por módulo: a partir de aquí, segunda persona.
CREATE TABLE umbral_aprobacion (
    tenant          text        NOT NULL,
    modulo          text        NOT NULL,
    -- En CÉNTIMOS enteros. 0 = todo pide aprobación; sin fila = ninguno.
    importe         bigint      NOT NULL,
    moneda          text        NOT NULL DEFAULT 'EUR',
    actualizado_por text        NOT NULL DEFAULT '',
    actualizado_en  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant, modulo),
    CONSTRAINT umbral_aprobacion_importe CHECK (importe >= 0),
    CONSTRAINT umbral_aprobacion_moneda CHECK (moneda ~ '^[A-Z]{3}$')
);

CREATE TABLE aprobacion (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    modelo        text        NOT NULL,
    registro_id   uuid        NOT NULL,
    -- El importe y su divisa, congelados en el momento de pedirla: si alguien
    -- edita el documento después, lo aprobado y lo que hay dejan de coincidir y
    -- se ve (la aprobación se invalida, ver `importe_al_pedir`).
    importe       bigint      NOT NULL,
    moneda        text        NOT NULL,
    estado        text        NOT NULL DEFAULT 'pendiente',
    solicitada_por text       NOT NULL,
    solicitada_en  timestamptz NOT NULL DEFAULT now(),
    -- El dueño del documento en el momento de pedirla: es el otro par de ojos
    -- que no puede ser el mismo.
    creada_por    text        NOT NULL DEFAULT '',
    resuelta_por  text        NOT NULL DEFAULT '',
    resuelta_en   timestamptz,
    motivo        text        NOT NULL DEFAULT '',

    CONSTRAINT aprobacion_estado CHECK (estado IN ('pendiente', 'aprobada', 'rechazada')),
    CONSTRAINT aprobacion_moneda CHECK (moneda ~ '^[A-Z]{3}$'),
    CONSTRAINT aprobacion_motivo CHECK (length(motivo) <= 1000)
);

-- Una sola solicitud VIVA por documento: pedirla dos veces no crea dos colas.
CREATE UNIQUE INDEX aprobacion_una_viva ON aprobacion (tenant, registro_id)
    WHERE estado = 'pendiente';
CREATE INDEX aprobacion_pendientes ON aprobacion (tenant, estado, solicitada_en DESC);
CREATE INDEX aprobacion_de_registro ON aprobacion (tenant, registro_id, solicitada_en DESC);

ALTER TABLE umbral_aprobacion ENABLE ROW LEVEL SECURITY;
ALTER TABLE umbral_aprobacion FORCE ROW LEVEL SECURITY;
ALTER TABLE aprobacion ENABLE ROW LEVEL SECURITY;
ALTER TABLE aprobacion FORCE ROW LEVEL SECURITY;

CREATE POLICY umbral_aprobacion_tenant_isolation ON umbral_aprobacion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY aprobacion_tenant_isolation ON aprobacion
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Literales, como en la 0009 y la 0011: el gate de fusión lee el SQL sin ejecutarlo.
CREATE POLICY umbral_aprobacion_platform_merge ON umbral_aprobacion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY aprobacion_platform_merge ON aprobacion
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
