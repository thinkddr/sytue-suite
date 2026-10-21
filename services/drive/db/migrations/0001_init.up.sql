-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base de Sytue Drive: jerarquía de carpetas/ficheros, compartición,
-- cuotas por tenant y outbox de eventos. Metadatos en PostgreSQL (única fuente
-- de verdad); los blobs viven en Object Storage S3 (no en la BD).
--
-- Multi-tenant: cada fila lleva `tenant` (slug). RLS deny-by-default aísla por
-- la variable de sesión `app.tenant` (la fija pg.SetTenant en cada transacción).

-- Necesario para gen_random_uuid() en builds sin la GUC por defecto.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Carpetas: árbol por tenant. La raíz de cada tenant tiene parent_id NULL y
-- name = '' (gestionada por la app, no borrable).
-- ---------------------------------------------------------------------------
CREATE TABLE folders (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    parent_id   uuid        REFERENCES folders (id) ON DELETE CASCADE,
    name        text        NOT NULL,
    is_root     boolean     NOT NULL DEFAULT false,
    -- group_folder marca carpetas compartidas a nivel de organización (group
    -- folders): visibles para todo el tenant según ACL de shares.
    is_group    boolean     NOT NULL DEFAULT false,
    owner_sub   text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz,
    CONSTRAINT folders_name_not_blank CHECK (is_root OR length(name) > 0)
);

-- Una sola raíz por tenant.
CREATE UNIQUE INDEX folders_one_root_per_tenant
    ON folders (tenant) WHERE is_root;

-- Nombres únicos entre hermanos vivos (case-insensitive), por tenant.
CREATE UNIQUE INDEX folders_unique_sibling_name
    ON folders (tenant, parent_id, lower(name))
    WHERE deleted_at IS NULL AND NOT is_root;

CREATE INDEX folders_parent_idx ON folders (tenant, parent_id) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Ficheros: metadatos. El contenido se guarda en S3 bajo storage_key (clave
-- relativa al prefijo de tenant que aplica platform/pkg/objectstore).
-- ---------------------------------------------------------------------------
CREATE TABLE files (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    folder_id     uuid        NOT NULL REFERENCES folders (id) ON DELETE CASCADE,
    name          text        NOT NULL,
    -- storage_key es la clave S3 (sin el prefijo de tenant, que lo añade el
    -- objectstore). Estable durante toda la vida del fichero.
    storage_key   text        NOT NULL,
    size_bytes    bigint      NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    content_type  text        NOT NULL DEFAULT 'application/octet-stream',
    -- etag/checksum del contenido (sha-256 hex) para integridad y dedupe futuro.
    checksum      text        NOT NULL DEFAULT '',
    owner_sub     text        NOT NULL DEFAULT '',
    version       integer     NOT NULL DEFAULT 1,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz,
    CONSTRAINT files_name_not_blank CHECK (length(name) > 0)
);

CREATE UNIQUE INDEX files_unique_name_in_folder
    ON files (tenant, folder_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE INDEX files_folder_idx ON files (tenant, folder_id) WHERE deleted_at IS NULL;
-- Búsqueda por nombre (trigram-friendly prefijo case-insensitive).
CREATE INDEX files_name_search_idx ON files (tenant, lower(name)) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Compartición: enlaces (link) y por sujeto (usuario/grupo de Sytue ID).
-- Aplica a un fichero o a una carpeta (exactamente uno).
-- ---------------------------------------------------------------------------
CREATE TABLE shares (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    file_id       uuid        REFERENCES files (id) ON DELETE CASCADE,
    folder_id     uuid        REFERENCES folders (id) ON DELETE CASCADE,
    -- kind: 'link' (público con token) | 'user' (subject) | 'group' (grupo Sytue ID).
    kind          text        NOT NULL CHECK (kind IN ('link', 'user', 'group')),
    -- subject: sub del usuario o slug del grupo (vacío para kind='link').
    subject       text        NOT NULL DEFAULT '',
    -- permission: 'read' | 'write'.
    permission    text        NOT NULL DEFAULT 'read' CHECK (permission IN ('read', 'write')),
    -- token: secreto del enlace público (NULL salvo kind='link').
    token         text,
    -- password_hash: hash opcional (Argon2/bcrypt) para enlaces protegidos.
    password_hash text,
    created_by    text        NOT NULL DEFAULT '',
    expires_at    timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    revoked_at    timestamptz,
    CONSTRAINT shares_one_target CHECK (
        (file_id IS NOT NULL AND folder_id IS NULL) OR
        (file_id IS NULL AND folder_id IS NOT NULL)
    ),
    CONSTRAINT shares_link_token CHECK (kind <> 'link' OR token IS NOT NULL),
    CONSTRAINT shares_subject_present CHECK (kind = 'link' OR length(subject) > 0)
);

CREATE UNIQUE INDEX shares_token_idx ON shares (token) WHERE token IS NOT NULL AND revoked_at IS NULL;
CREATE INDEX shares_file_idx ON shares (tenant, file_id) WHERE revoked_at IS NULL;
CREATE INDEX shares_folder_idx ON shares (tenant, folder_id) WHERE revoked_at IS NULL;
CREATE INDEX shares_subject_idx ON shares (tenant, subject) WHERE revoked_at IS NULL;

-- ---------------------------------------------------------------------------
-- Cuotas: una fila por tenant. used_bytes se mantiene transaccionalmente con
-- las altas/bajas de ficheros (ver store).
-- ---------------------------------------------------------------------------
CREATE TABLE quotas (
    tenant      text        PRIMARY KEY,
    limit_bytes bigint      NOT NULL CHECK (limit_bytes >= 0),
    used_bytes  bigint      NOT NULL DEFAULT 0 CHECK (used_bytes >= 0),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Outbox de eventos (sobre PostgreSQL; sin Redis/Valkey). DDL compartido de
-- platform/pkg/events, replicado aquí porque cada servicio gestiona su esquema.
-- ---------------------------------------------------------------------------
CREATE TABLE events_outbox (
    id           uuid        PRIMARY KEY,
    channel      text        NOT NULL,
    type         text        NOT NULL,
    tenant       text        NOT NULL DEFAULT '',
    payload      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    attempts     int         NOT NULL DEFAULT 0,
    last_error   text
);

CREATE INDEX events_outbox_unprocessed_idx
    ON events_outbox (created_at)
    WHERE processed_at IS NULL;

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por tenant deny-by-default (CONVENTIONS §10).
-- Las políticas comparan `tenant` con current_setting('app.tenant', true), que
-- fija pg.SetTenant LOCAL a cada transacción de negocio.
-- ---------------------------------------------------------------------------
ALTER TABLE folders ENABLE ROW LEVEL SECURITY;
ALTER TABLE files   ENABLE ROW LEVEL SECURITY;
ALTER TABLE shares  ENABLE ROW LEVEL SECURITY;
ALTER TABLE quotas  ENABLE ROW LEVEL SECURITY;

ALTER TABLE folders FORCE ROW LEVEL SECURITY;
ALTER TABLE files   FORCE ROW LEVEL SECURITY;
ALTER TABLE shares  FORCE ROW LEVEL SECURITY;
ALTER TABLE quotas  FORCE ROW LEVEL SECURITY;

CREATE POLICY folders_tenant_isolation ON folders
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY files_tenant_isolation ON files
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY shares_tenant_isolation ON shares
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY quotas_tenant_isolation ON quotas
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
