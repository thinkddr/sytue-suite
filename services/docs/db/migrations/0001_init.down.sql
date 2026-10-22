-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Reversión del esquema inicial de Sytue Docs.

-- Revocar privilegios y soltar el rol de aplicación (deny-by-default RLS).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'docs_app') THEN
        EXECUTE 'REVOKE ALL ON ALL TABLES IN SCHEMA public FROM docs_app';
        EXECUTE 'REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM docs_app';
        EXECUTE 'REVOKE ALL ON SCHEMA public FROM docs_app';
        EXECUTE 'REVOKE ALL ON FUNCTION docs_resolve_share(text) FROM docs_app';
        EXECUTE 'DROP ROLE docs_app';
    END IF;
END
$$;

DROP FUNCTION IF EXISTS docs_resolve_share(text);

DROP TABLE IF EXISTS events_outbox;
DROP TABLE IF EXISTS doc_blobs;
DROP TABLE IF EXISTS doc_comments;
DROP TABLE IF EXISTS doc_share_links;
DROP TABLE IF EXISTS doc_permissions;
DROP TABLE IF EXISTS doc_versions;

-- Romper la FK de documents -> doc_snapshots antes de soltar snapshots.
ALTER TABLE IF EXISTS documents DROP CONSTRAINT IF EXISTS documents_snapshot_fk;

DROP TABLE IF EXISTS doc_snapshots;
DROP TABLE IF EXISTS doc_operations;
DROP TABLE IF EXISTS documents;

DROP TYPE IF EXISTS blob_format;
DROP TYPE IF EXISTS principal_kind;
DROP TYPE IF EXISTS doc_role;
DROP TYPE IF EXISTS doc_kind;
