-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- «MI UNIDAD» PRIVADA POR PERSONA: el ámbito personal deja de ser compartido
-- por toda la organización.
--
-- Hasta la migración 0031, «Mi unidad» (`team_drive_id IS NULL AND channel_id IS NULL`)
-- tenía una sola carpeta raíz por tenant (`folders_one_root_per_tenant`) y la
-- política RLS solo comprobaba `tenant`, `team_drive_id` y `channel_id`. El
-- resultado era que cualquier persona de la organización veía, listaba, buscaba y
-- descargaba los ficheros personales de cualquier otra en «Mi unidad».
--
-- Con esta migración:
--   1. Cada persona (`owner_sub`) tiene su propia carpeta raíz (`is_root = true`)
--      dentro de su organización (`folders_one_root_per_owner`).
--   2. Las carpetas y ficheros preexistentes en la raíz compartida (`owner_sub = ''`)
--      cuyo `owner_sub` sea el de una persona se reubican bajo la raíz personal de
--      esa persona.
--   3. La política `folders_scope_isolation` / `files_scope_isolation` exige además,
--      cuando `team_drive_id IS NULL AND channel_id IS NULL`, que `owner_sub`
--      coincida con la GUC LOCAL `app.owner_sub` de la transacción.
--   4. Lo compartido con toda la organización o un equipo vive en Unidades de
--      equipo (`team_drive_id IS NOT NULL`) o se comparte explícitamente mediante
--      `shares` (usuario, grupo o enlace).

DROP INDEX folders_one_root_per_tenant;
CREATE UNIQUE INDEX folders_one_root_per_owner
    ON folders (tenant, owner_sub) WHERE is_root;

-- Backfill: crea la carpeta raíz personal de cada propietaria que ya tuviera
-- carpetas o ficheros en «Mi unidad» y reubica los hijos directos de la raíz
-- compartida (`owner_sub = ''`) bajo su raíz personal.
INSERT INTO folders (tenant, parent_id, name, is_root, owner_sub)
SELECT DISTINCT d.tenant, NULL::uuid, '', true, d.owner_sub
FROM (
    SELECT tenant, owner_sub FROM folders
    WHERE team_drive_id IS NULL AND channel_id IS NULL AND NOT is_root AND owner_sub <> ''
    UNION
    SELECT tenant, owner_sub FROM files
    WHERE team_drive_id IS NULL AND channel_id IS NULL AND owner_sub <> ''
) d
ON CONFLICT (tenant, owner_sub) WHERE is_root DO NOTHING;

UPDATE folders c
SET parent_id = r_new.id
FROM folders r_old, folders r_new
WHERE c.team_drive_id IS NULL AND c.channel_id IS NULL AND NOT c.is_root AND c.owner_sub <> ''
  AND r_old.id = c.parent_id AND r_old.tenant = c.tenant AND r_old.is_root AND r_old.owner_sub = ''
  AND r_new.tenant = c.tenant AND r_new.is_root AND r_new.owner_sub = c.owner_sub;

UPDATE files fi
SET folder_id = r_new.id
FROM folders r_old, folders r_new
WHERE fi.team_drive_id IS NULL AND fi.channel_id IS NULL AND fi.owner_sub <> ''
  AND r_old.id = fi.folder_id AND r_old.tenant = fi.tenant AND r_old.is_root AND r_old.owner_sub = ''
  AND r_new.tenant = fi.tenant AND r_new.is_root AND r_new.owner_sub = fi.owner_sub;

CREATE INDEX folders_personal_owner_idx ON folders (tenant, owner_sub)
    WHERE team_drive_id IS NULL AND channel_id IS NULL AND deleted_at IS NULL;
CREATE INDEX files_personal_owner_idx ON files (tenant, owner_sub)
    WHERE team_drive_id IS NULL AND channel_id IS NULL AND deleted_at IS NULL;

-- sytue:destructive-ok las dos políticas se reemplazan en el acto en la misma transacción para añadir el aislamiento por app.owner_sub en «Mi unidad».
DROP POLICY folders_scope_isolation ON folders;
CREATE POLICY folders_scope_isolation ON folders
    USING (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid
       AND (team_drive_id IS NOT NULL
         OR channel_id    IS NOT NULL
         OR owner_sub = coalesce(current_setting('app.owner_sub', true), '')))
    WITH CHECK (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid
       AND (team_drive_id IS NOT NULL
         OR channel_id    IS NOT NULL
         OR owner_sub = coalesce(current_setting('app.owner_sub', true), '')));

DROP POLICY files_scope_isolation ON files;
CREATE POLICY files_scope_isolation ON files
    USING (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid
       AND (team_drive_id IS NOT NULL
         OR channel_id    IS NOT NULL
         OR owner_sub = coalesce(current_setting('app.owner_sub', true), '')))
    WITH CHECK (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
       AND channel_id    IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid
       AND (team_drive_id IS NOT NULL
         OR channel_id    IS NOT NULL
         OR owner_sub = coalesce(current_setting('app.owner_sub', true), '')));
