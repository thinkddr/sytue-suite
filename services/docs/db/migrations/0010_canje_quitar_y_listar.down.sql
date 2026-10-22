-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP FUNCTION IF EXISTS docs_de_fuera(text, text, text, text, boolean, text, timestamptz, uuid, int);
-- docs_anfitrion vuelve al cuerpo de la 0009.
CREATE OR REPLACE FUNCTION docs_anfitrion(p_document_id uuid, p_sub text)
RETURNS text
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
STABLE
AS $$
    SELECT p.tenant
    FROM doc_permissions p
    JOIN documents d ON d.id = p.document_id AND d.deleted_at IS NULL
    LEFT JOIN doc_share_links l ON l.id = p.via_link
    WHERE p.document_id = p_document_id
      AND p.principal_kind = 'user'
      AND p.principal_id = p_sub
      AND p.externo
      AND (p.via_link IS NULL
           OR (l.revoked_at IS NULL AND (l.expires_at IS NULL OR l.expires_at > now())))
    LIMIT 1
$$;
DROP FUNCTION IF EXISTS docs_permiso_vivo(uuid, timestamptz);
-- Lo bloqueado se pierde: vuelve a poder canjear el enlace.
SELECT set_config('app.merge', 'on', true);
DELETE FROM doc_permissions WHERE bloqueado_desde IS NOT NULL;
ALTER TABLE doc_permissions DROP COLUMN IF EXISTS bloqueado_desde;
