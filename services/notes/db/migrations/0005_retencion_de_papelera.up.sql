-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- RETENCIÓN DE PAPELERA: política PERMISSIVE acotada a `app.maintenance='on'`.
--
-- Hoy la papelera de Notes no caduca. Una nota borrada se queda ahí para siempre:
-- ocupa disco, sigue siendo recuperable por quien la borró y —lo que importa—
-- sobrevive a cualquier plazo de conservación que la organización se haya dado.
-- Drive purga a los 30 días (`TRASH_RETENTION`); esto es lo mismo para Notes.
--
-- # Por qué hace falta una política
--
-- La purga la hace un worker: no tiene sesión ni organización, así que corre bajo
-- `pg.InMaintenanceTx`. Con FORCE ROW LEVEL SECURITY y sin `app.tenant` fijado,
-- la política de aislamiento evalúa a NULL y el DELETE afectaría a CERO filas —el
-- worker diría «purgadas 0» para siempre sin que nada fallara—. Es la misma
-- degradación silenciosa que tuvo el enlace público antes de la 0003.
--
-- # El alcance, otra vez estrecho
--
--   · Variable propia `app.maintenance`, la que fija `pg.InMaintenanceTx` y solo
--     ella. Distinta de `app.merge` y de `app.public_link`: ninguna transacción
--     existente gana alcance nuevo.
--   · Solo sobre `notes` y `notebooks`, que es lo que la papelera contiene. Las
--     etiquetas, los enlaces entre notas y la traza de auditoría NO la reciben:
--     no se borran por retención y dárselas sería ampliar el alcance por
--     simetría, que es como se cuelan los agujeros.
--   · Y aun con la variable puesta, quien la usa solo borra filas con
--     `deleted_at` vencido: el predicado va en la consulta del worker, no aquí.
--
-- # Lo que esta política NO da
--
-- No es un BYPASSRLS. Una transacción de negocio nunca fija `app.maintenance`,
-- así que el aislamiento por organización queda exactamente igual que antes para
-- todo lo que no sea el worker.

CREATE POLICY notes_mantenimiento ON notes
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY notebooks_mantenimiento ON notebooks
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- La purga busca por `deleted_at`, no por tenant: es la única consulta del
-- servicio que recorre TODAS las organizaciones, y sin índice haría un recorrido
-- completo de la tabla cada hora.
CREATE INDEX notes_papelera_idx     ON notes     (deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX notebooks_papelera_idx ON notebooks (deleted_at) WHERE deleted_at IS NOT NULL;
