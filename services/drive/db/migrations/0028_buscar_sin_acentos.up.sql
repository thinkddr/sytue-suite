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

-- El nombre, por su forma plegada. La búsqueda de retención legal (eDiscovery) usa
-- la misma regla sin índice propio: la acota la persona custodiada.
--
-- OJO, medido: con RLS forzada, Postgres NO usa índices de trigramas para el rol de
-- aplicación, porque LIKE e ILIKE no son LEAKPROOF y no pueden ir por delante de la
-- política. Ya pasaba con el índice sobre lower(name). El índice sirve a quien
-- consulta sin RLS; para el rol de aplicación, lo que acota es el tenant.
CREATE INDEX files_name_plegado_trgm_idx
    ON files USING gin (plegar(name) gin_trgm_ops)
    WHERE deleted_at IS NULL;
DROP INDEX IF EXISTS files_name_trgm_idx;
