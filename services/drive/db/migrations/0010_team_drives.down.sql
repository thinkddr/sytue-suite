-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte las unidades de equipo: restaura el aislamiento por-tenant original de
-- folders/files, quita la columna de ámbito y elimina las tablas de unidades.

-- Restaurar las políticas de aislamiento por tenant (deben quitarse ANTES de
-- soltar la columna team_drive_id, que la política de ámbito referencia).
DROP POLICY IF EXISTS folders_scope_isolation ON folders;
DROP POLICY IF EXISTS files_scope_isolation ON files;
CREATE POLICY folders_tenant_isolation ON folders
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY files_tenant_isolation ON files
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

DROP INDEX IF EXISTS folders_team_drive_idx;
DROP INDEX IF EXISTS files_team_drive_idx;
ALTER TABLE folders DROP COLUMN IF EXISTS team_drive_id;
ALTER TABLE files   DROP COLUMN IF EXISTS team_drive_id;

DROP TABLE IF EXISTS team_drive_members;
DROP TABLE IF EXISTS team_drives;
