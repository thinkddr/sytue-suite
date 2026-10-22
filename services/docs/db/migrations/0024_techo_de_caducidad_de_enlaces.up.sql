-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- TECHO DE CADUCIDAD DE LOS ENLACES (docs/security.md#matriz-acl, §5.5 F.4).
--
-- Un enlace de compartición podía no caducar nunca. Ahora cada organización tiene un
-- máximo de días de vida para sus enlaces: 365 si no ha dicho nada, el que fije quien
-- la administra, o 0 = «sin techo», que es el único modo de volver a tener enlaces sin
-- caducidad y exige esa decisión explícita.
--
-- El techo se MATERIALIZA en `expires_at`: así lo cumplen, sin tocarlas, todas las
-- comprobaciones que ya miraban la caducidad (`docs_permiso_vivo`, `docs_anfitrion`,
-- `ShareLink.Active`). Las dos puertas de entrada (`docs_resolve_share` y
-- `docs_canjear_enlace`) añaden además la regla al comprobar: un enlace sin caducidad
-- solo vale si su organización permite «sin techo».
CREATE TABLE doc_org_politica_enlaces (
    tenant     text        PRIMARY KEY,
    max_dias   integer     NOT NULL CHECK (max_dias BETWEEN 0 AND 3650),
    updated_by text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE doc_org_politica_enlaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE doc_org_politica_enlaces FORCE ROW LEVEL SECURITY;
CREATE POLICY doc_org_politica_enlaces_tenant_isolation ON doc_org_politica_enlaces
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY doc_org_politica_enlaces_platform_merge ON doc_org_politica_enlaces
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
-- Las funciones que resuelven un enlace sin sesión leen la política de SU organización.
CREATE POLICY doc_org_politica_enlaces_docs_definer ON doc_org_politica_enlaces FOR SELECT
    USING (current_setting('app.docs_definer', true) = 'on');

GRANT SELECT, INSERT, UPDATE, DELETE ON doc_org_politica_enlaces TO docs_app;

-- Cuándo la política acortó la vida de un enlace que ya existía: la interfaz lo avisa.
ALTER TABLE doc_share_links ADD COLUMN ajustado_por_politica_at timestamptz;

-- Los enlaces que ya existían NO se rompen ni se borran: los vivos que no cumplen el
-- techo por defecto (sin caducidad, o más allá de 365 días desde su creación) pasan a
-- caducar en su techo, y nunca antes de 30 días desde hoy. Quedan marcados.
-- `app.merge` es la única puerta que deja escribir filas de todas las organizaciones
-- bajo FORCE ROW LEVEL SECURITY; vale solo dentro de este bloque.
DO $$
BEGIN
    PERFORM set_config('app.merge', 'on', true);
    UPDATE doc_share_links
    SET expires_at = GREATEST(created_at + interval '365 days', now() + interval '30 days'),
        ajustado_por_politica_at = now()
    WHERE revoked_at IS NULL
      AND (expires_at IS NULL
           OR expires_at > GREATEST(created_at + interval '365 days', now() + interval '30 days'));
    PERFORM set_config('app.merge', 'off', true);
END $$;

-- Las dos puertas de entrada, con la regla al comprobar. Mismas firmas, dueño y
-- permisos (CREATE OR REPLACE los conserva); `app.docs_definer` se repite porque
-- reemplazar una función borra sus SET.
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
      AND (l.expires_at IS NOT NULL
           OR coalesce((SELECT p.max_dias FROM doc_org_politica_enlaces p WHERE p.tenant = l.tenant), 365) = 0)
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
      AND (l.expires_at IS NOT NULL
           OR coalesce((SELECT p.max_dias FROM doc_org_politica_enlaces p WHERE p.tenant = l.tenant), 365) = 0)
      AND d.deleted_at IS NULL
$$;
