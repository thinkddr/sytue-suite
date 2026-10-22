-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

CREATE INDEX IF NOT EXISTS documents_title_trgm_idx ON documents USING gin (title gin_trgm_ops);
DROP INDEX IF EXISTS documents_title_plegado_trgm_idx;
CREATE OR REPLACE FUNCTION docs_de_fuera(
    p_sub text, p_kind text, p_owner text, p_query text, p_starred boolean,
    p_source text, p_before_at timestamptz, p_before_id uuid, p_limit int)
RETURNS TABLE (id uuid, tenant text, kind doc_kind, title text, owner_sub text, seq bigint,
               snapshot_id uuid, starred_by text[], created_at timestamptz,
               updated_at timestamptz, source_ref text)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
STABLE
AS $$
    SELECT d.id, d.tenant, d.kind, d.title, d.owner_sub, d.seq, d.snapshot_id,
           d.starred_by, d.created_at, d.updated_at, coalesce(d.source_ref, '')
    FROM doc_permissions p
    JOIN documents d ON d.id = p.document_id AND d.deleted_at IS NULL
    WHERE p.principal_kind = 'user' AND p.principal_id = p_sub AND p.externo
      AND docs_permiso_vivo(p.via_link, p.bloqueado_desde)
      AND (p_kind = '' OR d.kind = p_kind::doc_kind)
      AND (p_owner = '' OR d.owner_sub = p_owner)
      AND (p_query = '' OR d.title ILIKE '%' || p_query || '%')
      AND (NOT p_starred OR d.starred_by @> ARRAY[p_sub]::text[])
      AND (p_source = '' OR d.source_ref = p_source)
      AND (d.updated_at, d.id) < (p_before_at, p_before_id)
    ORDER BY d.updated_at DESC, d.id DESC
    LIMIT least(p_limit, 200)
$$;

-- plegar() y unaccent se quedan: la versión anterior no los usa y retirarlos
-- obligaría a soltar antes cualquier índice que dependa de ellos.
