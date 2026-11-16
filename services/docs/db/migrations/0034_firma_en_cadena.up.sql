-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- «Pedir firma» a VARIAS personas, en orden, con recordatorios y caducidad (pieza
-- D-CERT-01 del plan de mejoras). La 0011 era de un destinatario; esto no cambia esa
-- fila, la encadena:
--
-- · `circuito`: las peticiones de una misma cadena lo comparten (NULL = suelta, como
--   hasta ahora). `orden` es su turno, desde 0. Solo la del turno está «pendiente»;
--   las siguientes esperan en «en_espera», no las ve nadie más que quien pidió y no
--   avisan a nadie. Al firmar una, la siguiente pasa a pendiente con el FICHERO
--   FIRMADO de la anterior: cada persona firma sobre las firmas de las de antes.
--   Rechazar o cancelar cierra la cadena entera.
-- · `caduca`: pasado ese momento, lo que siga abierto pasa a «caducada» (lo hace el
--   barrido de cada hora del servicio). NULL = no caduca.
-- · `recordar_dias` y `avisada`: cada cuántos días se le recuerda a quien tiene el
--   turno y cuándo se le avisó por última vez (al pedir, al llegarle el turno o al
--   recordárselo). 0 = sin recordatorios.
ALTER TABLE pdf_peticiones_firma
    ADD COLUMN circuito      uuid,
    ADD COLUMN orden         smallint    NOT NULL DEFAULT 0,
    ADD COLUMN caduca        timestamptz,
    ADD COLUMN recordar_dias smallint    NOT NULL DEFAULT 0,
    ADD COLUMN avisada       timestamptz NOT NULL DEFAULT now(),
    DROP CONSTRAINT pdf_peticiones_firma_estado_chk,
    ADD CONSTRAINT pdf_peticiones_firma_estado_chk
        CHECK (estado IN ('pendiente', 'en_espera', 'firmada', 'rechazada', 'cancelada', 'caducada')),
    ADD CONSTRAINT pdf_peticiones_firma_orden_chk CHECK (orden BETWEEN 0 AND 9),
    ADD CONSTRAINT pdf_peticiones_firma_recordar_chk CHECK (recordar_dias BETWEEN 0 AND 30),
    -- Esperar turno solo tiene sentido dentro de una cadena, y nunca la primera.
    ADD CONSTRAINT pdf_peticiones_firma_en_espera_chk
        CHECK (estado <> 'en_espera' OR (circuito IS NOT NULL AND orden > 0)),
    -- Caducada sin firmar, como rechazada o cancelada: no queda ningún token.
    ADD CONSTRAINT pdf_peticiones_firma_enlace_caducada_chk
        CHECK (estado <> 'caducada' OR enlace IS NULL);

-- Un turno por cadena.
CREATE UNIQUE INDEX pdf_peticiones_firma_circuito_uq
    ON pdf_peticiones_firma (tenant, circuito, orden) WHERE circuito IS NOT NULL;

-- El barrido mira TODAS las organizaciones para encontrar lo vencido y lo que toca
-- recordar, y solo eso: esta política es de LECTURA. Cada fila se cambia después en
-- una transacción de SU organización, con su RLS de siempre (el patrón de la 0014).
CREATE POLICY pdf_peticiones_firma_barrido ON pdf_peticiones_firma
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');

-- Lo abierto, por caducidad y por último aviso: el barrido no recorre la tabla entera.
CREATE INDEX pdf_peticiones_firma_abiertas_idx
    ON pdf_peticiones_firma (caduca) WHERE estado IN ('pendiente', 'en_espera') AND caduca IS NOT NULL;
CREATE INDEX pdf_peticiones_firma_recordar_idx
    ON pdf_peticiones_firma (avisada) WHERE estado = 'pendiente' AND recordar_dias > 0;
