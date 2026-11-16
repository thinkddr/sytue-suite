-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Sin cadenas: lo que esperaba turno se cancela y lo caducado queda como cancelado,
-- que son los estados que la 0011 entiende.
DROP INDEX IF EXISTS pdf_peticiones_firma_recordar_idx;
DROP INDEX IF EXISTS pdf_peticiones_firma_abiertas_idx;
DROP POLICY IF EXISTS pdf_peticiones_firma_barrido ON pdf_peticiones_firma;
DROP INDEX IF EXISTS pdf_peticiones_firma_circuito_uq;
-- La fila de cada organización, sin la de la sesión: la vía de la fusión (como la 0012).
SELECT set_config('app.merge', 'on', true);
UPDATE pdf_peticiones_firma SET estado = 'cancelada', resuelta = coalesce(resuelta, now())
    WHERE estado IN ('en_espera', 'caducada');
ALTER TABLE pdf_peticiones_firma
    DROP CONSTRAINT pdf_peticiones_firma_enlace_caducada_chk,
    DROP CONSTRAINT pdf_peticiones_firma_en_espera_chk,
    DROP CONSTRAINT pdf_peticiones_firma_recordar_chk,
    DROP CONSTRAINT pdf_peticiones_firma_orden_chk,
    DROP CONSTRAINT pdf_peticiones_firma_estado_chk,
    ADD CONSTRAINT pdf_peticiones_firma_estado_chk
        CHECK (estado IN ('pendiente', 'firmada', 'rechazada', 'cancelada')),
    DROP COLUMN avisada,
    DROP COLUMN recordar_dias,
    DROP COLUMN caduca,
    DROP COLUMN orden,
    DROP COLUMN circuito;
