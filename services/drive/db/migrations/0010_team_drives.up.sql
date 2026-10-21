-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Unidades de equipo (shared/team drives, estilo Google Shared Drives /
-- SharePoint document libraries).
--
-- Una unidad de equipo es propiedad de la ORGANIZACIÓN (el tenant), NO de un
-- usuario: su carpeta RAÍZ propia posee todo el contenido, de modo que cuando un
-- miembro se va, sus ficheros NO se van con él (continuidad). El acceso es por
-- MEMBRESÍA deny-by-default y por ROL (viewer|editor|manager).
--
-- Diseño del aislamiento (la pieza clave): las carpetas/ficheros de una unidad
-- viven en las MISMAS tablas folders/files, marcados con team_drive_id (NULL = «Mi
-- unidad» personal). El ámbito lo impone la RLS mediante la GUC de sesión
-- `app.team_drive`, no cada consulta: así el contenido de una unidad JAMÁS se
-- filtra por las rutas personales (Recientes/Búsqueda/navegación/mutaciones por id)
-- ni entre unidades, SIN tocar ninguna de esas consultas.

-- ---------------------------------------------------------------------------
-- Unidades de equipo y su membresía. Ambas aisladas por tenant (RLS).
-- ---------------------------------------------------------------------------
CREATE TABLE team_drives (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant         text        NOT NULL,
    name           text        NOT NULL CHECK (length(name) > 0),
    -- root_folder_id: la carpeta RAÍZ propia de la unidad (owner = la unidad, no
    -- un usuario). Está DESLIGADA del árbol personal (parent_id NULL, is_root
    -- false) para no aparecer en «Mi unidad».
    root_folder_id uuid        NOT NULL REFERENCES folders (id) ON DELETE CASCADE,
    created_by     text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX team_drives_tenant_idx ON team_drives (tenant);

CREATE TABLE team_drive_members (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    drive_id     uuid        NOT NULL REFERENCES team_drives (id) ON DELETE CASCADE,
    -- user_subject: el `sub` del usuario (Sytue ID).
    user_subject text        NOT NULL CHECK (length(user_subject) > 0),
    role         text        NOT NULL CHECK (role IN ('viewer', 'editor', 'manager')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
-- Un usuario, un rol por unidad.
CREATE UNIQUE INDEX team_drive_members_unique ON team_drive_members (drive_id, user_subject);
-- «Mis unidades de equipo»: recorrido indexado por sujeto (sin barrer, sin N+1).
CREATE INDEX team_drive_members_subject_idx ON team_drive_members (tenant, user_subject);
CREATE INDEX team_drive_members_drive_idx ON team_drive_members (drive_id);

-- ---------------------------------------------------------------------------
-- team_drive_id: marca de propiedad/ámbito en folders/files. NULL = «Mi unidad».
-- ---------------------------------------------------------------------------
ALTER TABLE folders ADD COLUMN team_drive_id uuid;
ALTER TABLE files   ADD COLUMN team_drive_id uuid;
-- Contenido vivo de una unidad, indexado (listar/gatear sin barrer la tabla).
CREATE INDEX folders_team_drive_idx ON folders (tenant, team_drive_id)
    WHERE team_drive_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX files_team_drive_idx ON files (tenant, team_drive_id)
    WHERE team_drive_id IS NOT NULL AND deleted_at IS NULL;

-- RLS de las tablas nuevas: aislamiento por tenant deny-by-default (como el resto).
ALTER TABLE team_drives        ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_drives        FORCE  ROW LEVEL SECURITY;
ALTER TABLE team_drive_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_drive_members FORCE  ROW LEVEL SECURITY;
CREATE POLICY team_drives_tenant_isolation ON team_drives
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY team_drive_members_tenant_isolation ON team_drive_members
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- ---------------------------------------------------------------------------
-- Reemplaza el aislamiento de folders/files por aislamiento tenant + ÁMBITO.
-- `app.team_drive` (GUC LOCAL a la transacción) selecciona el ámbito:
--   vacío/ausente → «Mi unidad» (team_drive_id IS NULL),
--   un uuid       → solo el contenido de ESA unidad (team_drive_id = uuid).
-- `IS NOT DISTINCT FROM` casa NULL con NULL; `nullif(...,'')` colapsa el valor
-- ausente/'' a NULL (SET LOCAL revierte a '' al terminar la tx: sin fugas entre
-- transacciones de una misma conexión del pool). Efecto: las rutas personales
-- nunca ven contenido de unidades, y una unidad solo ve lo suyo, sin tocar las
-- consultas. La política PERMISSIVE de mantenimiento (mig 0009) sigue OR-eándose
-- para el worker de auto-purga cross-tenant (ve la papelera de todas las unidades).
--
-- sytue:destructive-ok las dos políticas se sustituyen en el acto por *_scope_isolation
-- (mismo fichero, misma transacción): la tabla nunca queda sin aislamiento por tenant, y
-- el binario anterior —que nunca fija app.team_drive— sigue viendo «Mi unidad».
DROP POLICY folders_tenant_isolation ON folders;
CREATE POLICY folders_scope_isolation ON folders
    USING (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid)
    WITH CHECK (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid);

DROP POLICY files_tenant_isolation ON files;
CREATE POLICY files_scope_isolation ON files
    USING (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid)
    WITH CHECK (tenant = current_setting('app.tenant', true)
       AND team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid);
