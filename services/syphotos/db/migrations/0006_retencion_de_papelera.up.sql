-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- AUTO-PURGA DE LA PAPELERA: política de mantenimiento y su índice.
--
-- La papelera de Fotos se podía vaciar a mano, pero nada la vaciaba sola: lo
-- tirado se quedaba indefinidamente ocupando fila y derivadas. El worker que lo
-- recoge recorre TODAS las organizaciones —es lo que lo hace un worker y no una
-- petición—, así que en su transacción no hay `app.tenant` que fijar y la
-- política de aislamiento no le dejaría ver ni borrar NADA. Sin esta política,
-- el worker diría «purgadas 0» para siempre sin que nada fallara, que es la
-- peor forma de no funcionar.
--
-- El alcance es estrecho: variable propia `app.maintenance`, la que fija
-- `pg.InMaintenanceTx` y solo ella. Distinta de `app.public_link` y de
-- cualquier otra, así que ninguna transacción existente gana alcance nuevo.
-- Una transacción de negocio nunca la fija, y para todo lo que no sea el worker
-- el aislamiento por organización queda exactamente igual que antes.
--
-- DOS TABLAS, y la segunda por un motivo distinto de la primera.
--
-- `assets` porque es de donde se borra. `edits` porque ANTES de borrar hay que
-- LEER la huella de la receta de cada foto: es lo que nombra sus derivadas, y
-- sin ella el worker borraría filas y dejaría las imágenes huérfanas en el
-- almacén para siempre. Sin política, ese LEFT JOIN no devuelve nada y la
-- huella sale vacía sin que nada falle — que es como se descubrió.
--
-- `edits` solo para LEER: el worker nunca escribe recetas. Las recetas y la
-- pertenencia a álbumes se van en cascada con la foto, y la integridad
-- referencial se salta las políticas de fila por diseño del motor, así que para
-- BORRARLAS no hay que abrir nada.

CREATE POLICY assets_mantenimiento ON assets
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY edits_mantenimiento ON edits
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');

-- La purga busca por `deleted_at` y no por organización: es la única consulta
-- del servicio que las recorre todas, y sin índice haría un recorrido completo
-- de la tabla cada hora.
CREATE INDEX assets_papelera_idx ON assets (deleted_at) WHERE deleted_at IS NOT NULL;
