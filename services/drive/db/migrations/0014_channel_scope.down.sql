-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte el ámbito de canal: restaura las políticas de la mig 0010 (tenant +
-- unidad de equipo) y elimina la columna/índices. ATENCIÓN: al volver atrás, los
-- adjuntos de canal quedan otra vez en el ámbito personal —es decir, legibles por
-- todo el tenant—, y sus carpetas por canal aparecen como carpetas sueltas con
-- nombre uuid (parent_id NULL) que el explorador no lista (solo navega desde la
-- raíz). Bajar esta migración reabre la fuga; hacerlo solo para depurar.
DROP POLICY folders_scope_isolation ON folders;
CREATE POLICY folders_scope_isolation ON folders
    USING (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid)
    WITH CHECK (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid);

DROP POLICY files_scope_isolation ON files;
CREATE POLICY files_scope_isolation ON files
    USING (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid)
    WITH CHECK (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid);

DROP INDEX files_channel_idx;
DROP INDEX folders_channel_root_idx;
ALTER TABLE files   DROP COLUMN channel_id;
ALTER TABLE folders DROP COLUMN channel_id;
