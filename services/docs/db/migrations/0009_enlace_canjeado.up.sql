-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- «Abrir en el editor» desde la vista pública `/s/{token}`: CANJEAR un enlace de
-- editor o de comentarista convierte el enlace en un permiso de la persona que ha
-- iniciado sesión, también si es de OTRA organización (ADR 0052, sobre el 0040).
--
-- · `via_link`: el permiso nace del enlace y vive lo que viva el enlace. La lectura
--   de permisos solo cuenta los de enlaces sin revocar y sin caducar, así que
--   revocar el enlace quita también lo canjeado; y si se borra, se borra con él.
-- · `externo`: la persona es de otra organización. El documento, sus filas y su RLS
--   siguen en la organización anfitriona (como las conversaciones del 0040).
ALTER TABLE doc_permissions
    ADD COLUMN via_link uuid REFERENCES doc_share_links (id) ON DELETE CASCADE,
    ADD COLUMN externo  boolean NOT NULL DEFAULT false;

CREATE INDEX doc_permissions_externo_idx
    ON doc_permissions (principal_id, document_id) WHERE externo;

-- Canjear: lo mismo que `docs_resolve_share`, más el id del enlace (para `via_link`).
CREATE FUNCTION docs_canjear_enlace(p_token_hash text)
RETURNS TABLE (link_id uuid, document_id uuid, tenant text, role doc_role)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
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

-- La ÚNICA lectura entre organizaciones de lo canjeado: dado un documento y un
-- `sub`, la organización anfitriona SI esa persona tiene allí un permiso `externo`
-- vivo. No devuelve nada más que el slug, y nada de lo que no sea suyo.
CREATE FUNCTION docs_anfitrion(p_document_id uuid, p_sub text)
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

REVOKE ALL ON FUNCTION docs_canjear_enlace(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION docs_anfitrion(uuid, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION docs_canjear_enlace(text) TO docs_app;
GRANT EXECUTE ON FUNCTION docs_anfitrion(uuid, text) TO docs_app;
