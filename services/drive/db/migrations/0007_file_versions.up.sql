-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Historial de versiones de fichero (paridad Google Drive).
--
-- Cada vez que se reemplaza el CONTENIDO de un fichero (subida de una versión
-- nueva o restauración), la versión ANTERIOR se conserva como una fila de
-- file_versions que REFERENCIA su blob (no lo copia): la clave S3 de cada subida
-- es aleatoria y única, así que el blob previo ya era un objeto distinto que
-- hasta ahora quedaba huérfano al reemplazar. Ahora se registra en su lugar.
--
-- modified_by en files: el sub del ÚLTIMO que subió contenido (no el creador
-- original de owner_sub). Al hacer snapshot se copia como autor de esa versión,
-- de modo que el autor por-versión es fiel aunque varios editen el fichero.
ALTER TABLE files ADD COLUMN modified_by text NOT NULL DEFAULT '';

CREATE TABLE file_versions (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    file_id      uuid        NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    -- version: el número que tenía el fichero cuando este contenido era el actual
    -- (monotónico y único por fichero: cada versión se supera y se archiva una vez).
    version      integer     NOT NULL,
    -- storage_key: la clave S3 del blob de ESTA versión (referencia, no copia).
    storage_key  text        NOT NULL,
    size_bytes   bigint      NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    content_type text        NOT NULL DEFAULT 'application/octet-stream',
    checksum     text        NOT NULL DEFAULT '',
    -- author_sub: quién subió el contenido de esta versión (files.modified_by).
    author_sub   text        NOT NULL DEFAULT '',
    -- created_at: cuándo este contenido dejó de ser el actual (= updated_at del
    -- fichero en el instante del snapshot). Sirve de línea temporal del historial.
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- Unicidad (tenant, file_id, version) + índice de LISTADO keyset: el recorrido
-- «versiones de un fichero, más nueva primero» lo sirve un backward index scan de
-- este único índice (ORDER BY version DESC, WHERE version < cursor). Sin OFFSET,
-- sin ordenación en memoria (CONVENTIONS §0.8: índice para TODO WHERE/ORDER).
CREATE UNIQUE INDEX file_versions_file_version_idx
    ON file_versions (tenant, file_id, version);

-- Row-Level Security: aislamiento por tenant deny-by-default (como el resto de
-- tablas). Ninguna versión de un fichero de otro tenant es legible/restaurable.
ALTER TABLE file_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_versions FORCE  ROW LEVEL SECURITY;
CREATE POLICY file_versions_tenant_isolation ON file_versions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- ponytail: RETENCIÓN = todas las versiones, sin poda. Una purga futura (por nº
-- máximo de versiones o por antigüedad) recolectaría storage_key de las versiones
-- sobrantes para borrar sus blobs; el ON DELETE CASCADE ya limpia las FILAS al
-- purgar el fichero, pero sus BLOBS quedan para un GC de huérfanos posterior.
