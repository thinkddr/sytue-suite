-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte el aislamiento por persona en «Mi unidad», restaurando las políticas
-- de la migración 0014 y el índice de raíz única por tenant.

DROP POLICY folders_scope_isolation ON folders;
CREATE POLICY folders_scope_isolation ON folders
    USING (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid)
    WITH CHECK (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid);

DROP POLICY files_scope_isolation ON files;
CREATE POLICY files_scope_isolation ON files
    USING (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid)
    WITH CHECK (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid);

DROP INDEX files_personal_owner_idx;
DROP INDEX folders_personal_owner_idx;

INSERT INTO folders (tenant, parent_id, name, is_root, owner_sub)
SELECT DISTINCT tenant, NULL::uuid, '', true, ''
FROM folders
WHERE is_root AND owner_sub <> ''
ON CONFLICT (tenant, owner_sub) WHERE is_root DO NOTHING;

UPDATE folders c
SET parent_id = r_shared.id
FROM folders r_owner, folders r_shared
WHERE c.parent_id = r_owner.id
  AND r_owner.is_root AND r_owner.owner_sub <> ''
  AND r_shared.tenant = r_owner.tenant AND r_shared.is_root AND r_shared.owner_sub = '';

UPDATE files fi
SET folder_id = r_shared.id
FROM folders r_owner, folders r_shared
WHERE fi.folder_id = r_owner.id
  AND r_owner.is_root AND r_owner.owner_sub <> ''
  AND r_shared.tenant = r_owner.tenant AND r_shared.is_root AND r_shared.owner_sub = '';

DELETE FROM folders WHERE is_root AND owner_sub <> '';

DROP INDEX folders_one_root_per_owner;
CREATE UNIQUE INDEX folders_one_root_per_tenant
    ON folders (tenant) WHERE is_root;
