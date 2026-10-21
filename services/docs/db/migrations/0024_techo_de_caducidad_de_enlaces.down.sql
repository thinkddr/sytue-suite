-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Bajar esto quita el techo y la marca de ajuste. Las caducidades que la política ya
-- acortó se QUEDAN como están: no se sabe cuál era la anterior, y alargar enlaces al
-- deshacer una migración sería abrir lo que se cerró.
CREATE OR REPLACE FUNCTION docs_resolve_share(p_token_hash text)
RETURNS TABLE (document_id uuid, tenant text, role doc_role)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
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

CREATE OR REPLACE FUNCTION docs_canjear_enlace(p_token_hash text)
RETURNS TABLE (link_id uuid, document_id uuid, tenant text, role doc_role)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
STABLE
AS $$
    SELECT l.id, l.document_id, l.tenant, l.role
    FROM doc_share_links l
    JOIN documents d ON d.id = l.document_id
    WHERE l.token_hash = p_token_hash
      AND l.revoked_at IS NULL
      AND (l.expires_at IS NULL OR l.expires_at > now())
      AND d.deleted_at IS NULL
$$;

ALTER TABLE doc_share_links DROP COLUMN IF EXISTS ajustado_por_politica_at;
DROP TABLE IF EXISTS doc_org_politica_enlaces;
