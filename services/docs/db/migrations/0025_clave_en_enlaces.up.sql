-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- CONTRASEÑA OPCIONAL EN LOS ENLACES DE COMPARTIR (docs/security.md#matriz-acl, §5.5 F.1).
--
-- Como los enlaces públicos de Drive: quien crea el enlace puede pedir una contraseña
-- además del token. Se guarda SOLO su hash (Argon2id con sal, formato PHC,
-- `platform/pkg/clave`); la contraseña no se guarda ni se devuelve nunca.
ALTER TABLE doc_share_links ADD COLUMN password_hash text;

-- La contraseña de un enlace VIVO, por el hash de su token. Sin sesión y entre
-- organizaciones, como `docs_resolve_share`, y con sus mismas condiciones (no revocado,
-- no caducado, techo de la organización, documento vivo): de un enlace que no vale no se
-- dice ni que existe. `password_hash` vacío = el enlace no pide contraseña.
-- Función nueva, y no una columna más en `docs_resolve_share`, porque cambiar el tipo
-- que devuelve una función obliga a borrarla.
CREATE FUNCTION docs_enlace_clave(p_token_hash text)
RETURNS TABLE (link_id uuid, password_hash text)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET app.docs_definer = 'on'
STABLE
AS $$
    SELECT l.id, coalesce(l.password_hash, '')
    FROM doc_share_links l
    JOIN documents d ON d.id = l.document_id
    WHERE l.token_hash = p_token_hash
      AND l.revoked_at IS NULL
      AND (l.expires_at IS NULL OR l.expires_at > now())
      AND (l.expires_at IS NOT NULL
           OR coalesce((SELECT p.max_dias FROM doc_org_politica_enlaces p WHERE p.tenant = l.tenant), 365) = 0)
      AND d.deleted_at IS NULL
$$;

REVOKE ALL ON FUNCTION docs_enlace_clave(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION docs_enlace_clave(text) TO docs_app;
