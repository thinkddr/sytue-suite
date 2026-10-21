-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- BUSCAR SIN ACENTOS NI MAYÚSCULAS: «reunion» encuentra «reunión», «Muller» encuentra
-- «Müller», y al revés. `plegar(text)` es el espejo SQL de `buscar.Plegar`
-- (apps/nucleo/buscar) y de `plegar` de @sytue/i18n; los tres se prueban con los
-- mismos casos (apps/nucleo/buscar/testdata/casos.json).
--
-- unaccent es contrib estándar de PostgreSQL (como pg_trgm) y extensión de confianza
-- desde la 13: la crea el dueño de la base sin superusuario. Su función es STABLE
-- solo porque resuelve el diccionario por nombre; aquí va fijado por OID al crear la
-- función (cuerpo SQL estándar, que se analiza al crearla), así que IMMUTABLE es
-- verdad y se puede indexar. `pg_c_utf8` (PostgreSQL 17+) da las minúsculas de
-- Unicode sea cual sea la configuración regional de la base.
CREATE EXTENSION IF NOT EXISTS unaccent;
CREATE OR REPLACE FUNCTION plegar(t text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
    RETURN lower(unaccent('unaccent'::regdictionary, t) COLLATE pg_c_utf8);

-- El título, por su forma plegada.
CREATE INDEX documents_title_plegado_trgm_idx ON documents USING gin (plegar(title) gin_trgm_ops);
DROP INDEX IF EXISTS documents_title_trgm_idx;

-- Lo canjeado de otras organizaciones se busca con la misma regla que lo propio.
-- CREATE OR REPLACE rehace también sus SET: el de app.docs_definer (0013) va aquí.
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
      AND (p_query = '' OR plegar(d.title) LIKE '%' || plegar(p_query) || '%')
      AND (NOT p_starred OR d.starred_by @> ARRAY[p_sub]::text[])
      AND (p_source = '' OR d.source_ref = p_source)
      AND (d.updated_at, d.id) < (p_before_at, p_before_id)
    ORDER BY d.updated_at DESC, d.id DESC
    LIMIT least(p_limit, 200)
$$;
