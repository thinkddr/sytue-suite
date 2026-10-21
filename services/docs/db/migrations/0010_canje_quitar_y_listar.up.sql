-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Lo que dejó la revisión de seguridad del canje (ADR 0052):
--
-- 1. QUITAR a alguien que entró por un enlace sirve de algo. Antes se borraba su fila y
--    volvía a canjear el mismo enlace al momento. Ahora la fila se queda BLOQUEADA desde
--    esa hora: no cuenta como permiso y no se le canjea ningún enlace que ya existiera
--    entonces —tampoco otro enlace vivo del mismo documento—. Uno NUEVO del dueño, sí.
-- 2. «¿Cuenta este permiso?» se escribe UNA vez (`docs_permiso_vivo`) y no en cada
--    consulta: lo canjeado de un enlace revocado o caducado no cuenta, ni lo bloqueado.
-- 3. La persona de fuera ve en su lista los documentos que canjeó (`docs_de_fuera`).
ALTER TABLE doc_permissions ADD COLUMN bloqueado_desde timestamptz;

CREATE FUNCTION docs_permiso_vivo(p_via_link uuid, p_bloqueado_desde timestamptz)
RETURNS boolean
LANGUAGE sql
STABLE
SET search_path = pg_catalog, public
AS $$
    SELECT p_bloqueado_desde IS NULL AND (
        p_via_link IS NULL
        OR EXISTS (SELECT 1 FROM doc_share_links l
                   WHERE l.id = p_via_link AND l.revoked_at IS NULL
                     AND (l.expires_at IS NULL OR l.expires_at > now()))
    )
$$;

-- La 0009 la creó; aquí solo cambia su cuerpo para usar la regla común (y no ver lo
-- bloqueado). Misma firma, misma superficie.
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
    WHERE p.document_id = p_document_id
      AND p.principal_kind = 'user'
      AND p.principal_id = p_sub
      AND p.externo
      AND docs_permiso_vivo(p.via_link, p.bloqueado_desde)
    LIMIT 1
$$;

-- Los documentos de OTRAS organizaciones en los que `p_sub` es externo con un permiso
-- vivo, con los mismos filtros y el mismo cursor que la lista de su organización. Es
-- la segunda lectura entre organizaciones de lo canjeado y devuelve solo lo que ya
-- abre: los metadatos de la lista de esos documentos, y nada de nadie más.
CREATE FUNCTION docs_de_fuera(
    p_sub text, p_kind text, p_owner text, p_query text, p_starred boolean,
    p_source text, p_before_at timestamptz, p_before_id uuid, p_limit int)
RETURNS TABLE (id uuid, tenant text, kind doc_kind, title text, owner_sub text, seq bigint,
               snapshot_id uuid, starred_by text[], created_at timestamptz,
               updated_at timestamptz, source_ref text)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
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

REVOKE ALL ON FUNCTION docs_permiso_vivo(uuid, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION docs_de_fuera(text, text, text, text, boolean, text, timestamptz, uuid, int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION docs_permiso_vivo(uuid, timestamptz) TO docs_app;
GRANT EXECUTE ON FUNCTION docs_de_fuera(text, text, text, text, boolean, text, timestamptz, uuid, int) TO docs_app;
