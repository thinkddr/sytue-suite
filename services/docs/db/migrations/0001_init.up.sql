-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Docs/Sheets/Slides — esquema inicial.
-- Ofimática colaborativa Sytue-native (clean-room sobre OOXML/ODF y CRDT propio).
-- Datastore ÚNICO: PostgreSQL (estado, operaciones CRDT, versiones, permisos,
-- presencia efímera y outbox de eventos). Los blobs/exports (OOXML/ODF) viven
-- en Object Storage S3 — aquí solo guardamos la referencia (clave + metadatos).
--
-- Multi-tenant: aislamiento por fila vía RLS con `current_setting('app.tenant')`
-- (ver platform/pkg/pg.SetTenant). Toda tabla con datos de tenant lleva columna
-- `tenant` y política RLS deny-by-default.

-- ---------------------------------------------------------------------------
-- Extensiones
-- ---------------------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()

-- ---------------------------------------------------------------------------
-- Enums de dominio
-- ---------------------------------------------------------------------------

-- Tipo de documento ofimático.
CREATE TYPE doc_kind AS ENUM ('document', 'spreadsheet', 'presentation');

-- Rol de un principal (usuario o grupo) sobre un documento.
--   viewer   -> lectura
--   commenter-> lectura + comentarios
--   editor   -> lectura/escritura colaborativa
--   owner    -> control total (incluye gestión de permisos y borrado)
CREATE TYPE doc_role AS ENUM ('viewer', 'commenter', 'editor', 'owner');

-- Naturaleza del principal al que se concede acceso.
CREATE TYPE principal_kind AS ENUM ('user', 'group', 'tenant', 'public');

-- Formato de export/import del blob materializado en S3.
CREATE TYPE blob_format AS ENUM (
    'sytue-crdt',   -- snapshot nativo del estado CRDT (interno)
    'ooxml',        -- docx/xlsx/pptx (OOXML, ECMA-376)
    'odf',          -- odt/ods/odp (OpenDocument)
    'pdf',          -- export de solo lectura
    'html',         -- export web
    'markdown'      -- export texto enriquecido ligero
);

-- ---------------------------------------------------------------------------
-- documents — metadatos del documento (la fuente de verdad del estado vive en
-- las operaciones CRDT + snapshots). Soft-delete con deleted_at (papelera).
-- ---------------------------------------------------------------------------
CREATE TABLE documents (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    kind          doc_kind    NOT NULL,
    title         text        NOT NULL DEFAULT 'Sin título',
    owner_sub     text        NOT NULL,            -- "sub" del creador (Sytue ID)
    -- Vector de versión lógica: nº de operación más alto aplicado/secuenciado.
    -- Sirve de optimistic concurrency y de cursor para clientes que reconectan.
    seq           bigint      NOT NULL DEFAULT 0,
    -- Referencia al último snapshot consolidado (acelera la carga inicial).
    snapshot_id   uuid,
    starred_by    text[]      NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);

CREATE INDEX documents_tenant_idx       ON documents (tenant) WHERE deleted_at IS NULL;
CREATE INDEX documents_owner_idx        ON documents (tenant, owner_sub) WHERE deleted_at IS NULL;
CREATE INDEX documents_updated_idx      ON documents (tenant, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX documents_title_trgm_idx   ON documents (tenant, lower(title));

-- ---------------------------------------------------------------------------
-- doc_operations — log inmutable de operaciones CRDT (append-only).
-- Cada operación lleva un identificador CRDT global ordenable
-- (lamport, actor) que garantiza convergencia con conmutatividad de inserción
-- y tombstone de borrado. El servidor solo SECUENCIA y persiste; el merge es
-- determinista en cliente y servidor (RGA sobre identidades densas).
-- ---------------------------------------------------------------------------
CREATE TABLE doc_operations (
    document_id  uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    seq          bigint      NOT NULL,             -- nº de secuencia servidor (monótono por doc)
    tenant       text        NOT NULL,
    actor        text        NOT NULL,             -- id de actor CRDT (sesión/cliente)
    author_sub   text        NOT NULL,             -- "sub" del autor (auditoría)
    lamport      bigint      NOT NULL,             -- reloj lógico Lamport del cliente
    -- Operación CRDT serializada (insert/delete/format/cell/...); el modelo
    -- concreto depende de doc_kind y lo interpreta internal/domain.
    op           jsonb       NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (document_id, seq)
);

CREATE INDEX doc_operations_doc_seq_idx ON doc_operations (document_id, seq);
CREATE INDEX doc_operations_tenant_idx  ON doc_operations (tenant);

-- ---------------------------------------------------------------------------
-- doc_snapshots — estado CRDT consolidado en un punto (acelera carga inicial).
-- El estado pequeño se guarda inline (jsonb); los grandes se materializan en S3
-- y aquí queda solo blob_key. Una de las dos columnas estará poblada.
-- ---------------------------------------------------------------------------
CREATE TABLE doc_snapshots (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id  uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    tenant       text        NOT NULL,
    seq          bigint      NOT NULL,             -- seq del doc cubierto por el snapshot
    state        jsonb,                            -- estado CRDT inline (si es pequeño)
    blob_key     text,                             -- clave S3 (si es grande)
    size_bytes   bigint      NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT doc_snapshots_state_or_blob
        CHECK (state IS NOT NULL OR blob_key IS NOT NULL)
);

CREATE INDEX doc_snapshots_doc_idx ON doc_snapshots (document_id, seq DESC);

ALTER TABLE documents
    ADD CONSTRAINT documents_snapshot_fk
    FOREIGN KEY (snapshot_id) REFERENCES doc_snapshots (id) ON DELETE SET NULL;

-- ---------------------------------------------------------------------------
-- doc_versions — historial de versiones nombradas/automáticas (restaurables).
-- Apunta a un snapshot inmutable; permite "ver historial" y "restaurar".
-- ---------------------------------------------------------------------------
CREATE TABLE doc_versions (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id  uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    tenant       text        NOT NULL,
    snapshot_id  uuid        NOT NULL REFERENCES doc_snapshots (id) ON DELETE CASCADE,
    label        text,                             -- nombre opcional (hito manual)
    author_sub   text        NOT NULL,
    seq          bigint      NOT NULL,
    auto         boolean     NOT NULL DEFAULT true, -- automática vs nombrada por usuario
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX doc_versions_doc_idx ON doc_versions (document_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- doc_permissions — ACL por documento. deny-by-default: sin fila = sin acceso
-- (salvo el owner_sub del documento, que siempre tiene rol owner).
-- principal_id: sub (user), slug de grupo (group), slug de tenant (tenant),
-- vacío para principal_kind='public' (enlace compartido).
-- ---------------------------------------------------------------------------
CREATE TABLE doc_permissions (
    document_id    uuid           NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    tenant         text           NOT NULL,
    principal_kind principal_kind NOT NULL,
    principal_id   text           NOT NULL DEFAULT '',
    role           doc_role       NOT NULL,
    granted_by     text           NOT NULL,
    created_at     timestamptz    NOT NULL DEFAULT now(),
    PRIMARY KEY (document_id, principal_kind, principal_id)
);

CREATE INDEX doc_permissions_principal_idx
    ON doc_permissions (tenant, principal_kind, principal_id);

-- ---------------------------------------------------------------------------
-- doc_share_links — enlaces de compartición públicos/firmados (token opaco).
-- El token se guarda hasheado (sha-256, hex); nunca en claro.
-- ---------------------------------------------------------------------------
CREATE TABLE doc_share_links (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id  uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    tenant       text        NOT NULL,
    token_hash   text        NOT NULL UNIQUE,      -- sha-256 hex del token
    role         doc_role    NOT NULL DEFAULT 'viewer',
    created_by   text        NOT NULL,
    expires_at   timestamptz,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX doc_share_links_doc_idx ON doc_share_links (document_id);

-- ---------------------------------------------------------------------------
-- doc_comments — comentarios anclados a un rango/celda/objeto del documento.
-- El ancla es una referencia CRDT estable (no un offset), para sobrevivir a
-- ediciones concurrentes. Hilos vía parent_id.
-- ---------------------------------------------------------------------------
CREATE TABLE doc_comments (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id  uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    tenant       text        NOT NULL,
    parent_id    uuid        REFERENCES doc_comments (id) ON DELETE CASCADE,
    author_sub   text        NOT NULL,
    anchor       jsonb       NOT NULL DEFAULT '{}'::jsonb, -- ancla CRDT estable
    body         text        NOT NULL,
    resolved     boolean     NOT NULL DEFAULT false,
    resolved_by  text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz
);

CREATE INDEX doc_comments_doc_idx ON doc_comments (document_id, created_at)
    WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- doc_blobs — exports/imports materializados en S3 (OOXML/ODF/PDF/...).
-- Solo metadatos + clave S3; el binario vive en Object Storage (bucket docs-blobs).
-- ---------------------------------------------------------------------------
CREATE TABLE doc_blobs (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id  uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    tenant       text        NOT NULL,
    format       blob_format NOT NULL,
    blob_key     text        NOT NULL,             -- clave S3 (con prefijo de tenant)
    seq          bigint      NOT NULL,             -- seq del doc en el momento del export
    size_bytes   bigint      NOT NULL DEFAULT 0,
    content_type text        NOT NULL DEFAULT 'application/octet-stream',
    created_by   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX doc_blobs_doc_idx ON doc_blobs (document_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- events_outbox — bus de eventos sobre PostgreSQL (sin Redis/Valkey).
-- Idéntico al DDL compartido de platform/pkg/events; cada servicio lo posee.
-- ---------------------------------------------------------------------------
CREATE TABLE events_outbox (
    id           uuid PRIMARY KEY,
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
-- Resolución de enlace de compartición público (cross-tenant controlada).
-- Un enlace público se resuelve por su token (hash) SIN conocer aún el tenant.
-- En lugar de conceder lecturas cross-tenant a la aplicación, exponemos UNA
-- función SECURITY DEFINER de superficie mínima: dado el hash del token, si el
-- enlace está activo (no revocado ni caducado) y el documento no está borrado,
-- devuelve solo (document_id, tenant, role). No filtra datos del documento.
-- La aplicación usa el `tenant` devuelto para fijar `app.tenant` y operar bajo
-- RLS normal a partir de ahí.
-- ---------------------------------------------------------------------------
CREATE FUNCTION docs_resolve_share(p_token_hash text)
RETURNS TABLE (document_id uuid, tenant text, role doc_role)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
STABLE
AS $$
    SELECT l.document_id, l.tenant, l.role
    FROM doc_share_links l
    JOIN documents d ON d.id = l.document_id
    WHERE l.token_hash = p_token_hash
      AND l.revoked_at IS NULL
      AND (l.expires_at IS NULL OR l.expires_at > now())
      AND d.deleted_at IS NULL
$$;

-- ---------------------------------------------------------------------------
-- Row-Level Security (multi-tenant, deny-by-default).
-- `app.tenant` lo fija platform/pkg/pg.SetTenant por transacción. El rol de la
-- aplicación NO es superusuario ni BYPASSRLS en producción.
-- ---------------------------------------------------------------------------
ALTER TABLE documents       ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_operations  ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_snapshots   ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_versions    ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_share_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_comments    ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_blobs       ENABLE ROW LEVEL SECURITY;

ALTER TABLE documents       FORCE ROW LEVEL SECURITY;
ALTER TABLE doc_operations  FORCE ROW LEVEL SECURITY;
ALTER TABLE doc_snapshots   FORCE ROW LEVEL SECURITY;
ALTER TABLE doc_versions    FORCE ROW LEVEL SECURITY;
ALTER TABLE doc_permissions FORCE ROW LEVEL SECURITY;
ALTER TABLE doc_share_links FORCE ROW LEVEL SECURITY;
ALTER TABLE doc_comments    FORCE ROW LEVEL SECURITY;
ALTER TABLE doc_blobs       FORCE ROW LEVEL SECURITY;

-- Política única por tabla: la fila debe pertenecer al tenant de la sesión.
CREATE POLICY documents_tenant_isolation       ON documents
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_operations_tenant_isolation   ON doc_operations
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_snapshots_tenant_isolation    ON doc_snapshots
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_versions_tenant_isolation      ON doc_versions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_permissions_tenant_isolation   ON doc_permissions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_share_links_tenant_isolation   ON doc_share_links
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_comments_tenant_isolation      ON doc_comments
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_blobs_tenant_isolation         ON doc_blobs
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- ---------------------------------------------------------------------------
-- Rol de aplicación (deny-by-default reforzado por RLS).
--
-- Las políticas RLS de arriba SOLO se ejercen sobre roles NO superusuario y SIN
-- BYPASSRLS: un superusuario (o el dueño de la tabla sin FORCE) las ignora. En
-- producción la aplicación se conecta con un rol así, pero para que el
-- aislamiento sea INDEPENDIENTE de cómo se conecte el pool (p. ej. superusuario
-- en dev/CI), el store hace `SET LOCAL ROLE docs_app` en cada transacción de
-- tenant. Este rol no puede iniciar sesión, no hereda privilegios de otros
-- roles, y NO tiene BYPASSRLS: bajo él, las políticas de tenant SIEMPRE aplican.
--
-- `SET ROLE` exige que el rol conectado sea miembro de docs_app (los
-- superusuarios lo son implícitamente). Concedemos la membresía al rol que
-- aplica esta migración (el dueño de las tablas) para dev/CI; en producción el
-- despliegue debe `GRANT docs_app TO <rol_de_la_app>`.
-- ---------------------------------------------------------------------------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'docs_app') THEN
        CREATE ROLE docs_app NOLOGIN NOINHERIT NOBYPASSRLS;
    END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO docs_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO docs_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO docs_app;
GRANT EXECUTE ON FUNCTION docs_resolve_share(text) TO docs_app;

-- Membresía para el dueño de las migraciones (dev/CI). Idempotente.
DO $$
BEGIN
    EXECUTE format('GRANT docs_app TO %I', current_user);
END
$$;
