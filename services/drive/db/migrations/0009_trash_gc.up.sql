-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Auto-purga de papelera: un worker periódico borra DEFINITIVAMENTE los
-- elementos en papelera cuya retención venció (deleted_at < now()-retención).
-- El worker es CROSS-TENANT (no tiene el `sub` de ningún usuario) y CROSS-RÉPLICA
-- (varias instancias reclaman con FOR UPDATE SKIP LOCKED sin purgar lo mismo dos
-- veces). Esta migración le da las dos piezas que necesita para escalar y ser
-- seguro:
--
--  1) Política PERMISSIVE de mantenimiento (`app.maintenance='on'`, sin BYPASSRLS)
--     sobre las tablas que la purga toca, EN PARALELO a la política de aislamiento
--     por tenant (que sigue rigiendo las peticiones de usuario). El worker reclama
--     y purga bajo `app.maintenance`, fijando el tenant EXPLÍCITO de cada fila
--     reclamada, así que el aislamiento por tenant se mantiene: una purga solo toca
--     datos de su propio tenant. La guarda de blob compartido (blobReferenced /
--     markOrphanedBlobs, que barren files+file_versions por storage_key) SOLO es
--     correcta si el worker VE esas filas cross-tenant; de ahí que file_versions
--     también reciba la política (si no, una versión viva quedaría invisible y su
--     blob se borraría por error). quotas la recibe para liberar cuota al purgar.
--
--  2) Índice parcial por (deleted_at) para que la selección de caducados sea un
--     index scan del subconjunto pequeño «en papelera» (no un seq-scan de la tabla
--     entera), ordenado por antigüedad, cross-tenant. Los índices de papelera de la
--     mig 0004 son (tenant, deleted_at): sirven el listado por-tenant, pero no la
--     selección global ordenada del worker.

-- 1) Políticas PERMISSIVE de mantenimiento (se OR-ean con las de aislamiento).
CREATE POLICY folders_maintenance ON folders
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY files_maintenance ON files
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY quotas_maintenance ON quotas
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY file_versions_maintenance ON file_versions
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- 2) Índices parciales de selección de caducados (solo filas en papelera).
CREATE INDEX folders_gc_idx ON folders (deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX files_gc_idx   ON files   (deleted_at) WHERE deleted_at IS NOT NULL;
