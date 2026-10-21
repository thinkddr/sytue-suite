-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Recientes + Favoritos (paridad Google Drive/Dropbox).
--
-- «Recientes» lista los ficheros vivos del tenant por reciente-primero
-- (updated_at DESC). Ningún índice previo cubría updated_at (todos son por
-- nombre/carpeta o de papelera), así que ese listado sería un seq-scan + sort de
-- todos los ficheros vivos del tenant. Añadimos un índice keyset que sirve el
-- recorrido (tenant, updated_at DESC, id DESC) sin OFFSET ni ordenación en
-- memoria (CONVENTIONS §0.8: índice para TODO WHERE/ORDER, keyset sin N+1).
CREATE INDEX files_recent_idx
    ON files (tenant, updated_at DESC, id DESC)
    WHERE deleted_at IS NULL;

-- «Favoritos» es una marca PERSONAL (por-usuario, no por-tenant) sobre un fichero
-- o una carpeta. Una fila por (tenant, subject, objetivo); el objetivo es
-- exactamente uno de file_id / folder_id (CHECK). ON DELETE CASCADE limpia la
-- marca cuando el objetivo se PURGA (borrado en duro); el borrado suave NO
-- cascada: un favorito en la papelera se oculta al listar (JOIN a deleted_at IS
-- NULL) y revive al restaurar.
CREATE TABLE favorites (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text        NOT NULL,
    -- subject: sub del usuario de Sytue ID dueño de la marca.
    subject    text        NOT NULL,
    file_id    uuid        REFERENCES files (id)   ON DELETE CASCADE,
    folder_id  uuid        REFERENCES folders (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT favorites_one_target CHECK (
        (file_id IS NOT NULL AND folder_id IS NULL) OR
        (file_id IS NULL AND folder_id IS NOT NULL)
    )
);

-- Idempotencia del toggle: un usuario no marca dos veces el mismo objetivo.
CREATE UNIQUE INDEX favorites_unique_file
    ON favorites (tenant, subject, file_id) WHERE file_id IS NOT NULL;
CREATE UNIQUE INDEX favorites_unique_folder
    ON favorites (tenant, subject, folder_id) WHERE folder_id IS NOT NULL;

-- Listado por usuario, reciente-primero (created_at DESC).
CREATE INDEX favorites_owner_idx ON favorites (tenant, subject, created_at DESC);

-- Row-Level Security: aislamiento por tenant deny-by-default (como el resto de
-- tablas). El filtro por `subject` es de aplicación (no de RLS).
ALTER TABLE favorites ENABLE ROW LEVEL SECURITY;
ALTER TABLE favorites FORCE  ROW LEVEL SECURITY;
CREATE POLICY favorites_tenant_isolation ON favorites
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
