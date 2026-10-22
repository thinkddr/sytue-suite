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

-- Texto libre: un tsvector GUARDADO sobre la forma plegada. Con RLS forzada el rol de
-- aplicación no usa el índice (el operador @@ no es LEAKPROOF) y cada búsqueda
-- calculaba to_tsvector() de TODO el buzón; guardado, se calcula una vez al escribir.
-- Remitente y asunto (from:/subject:) usan plegar() al vuelo: son cadenas cortas.
ALTER TABLE messages ADD COLUMN busqueda tsvector GENERATED ALWAYS AS (
    to_tsvector('simple', plegar(coalesce(subject,'') || ' ' || coalesce(from_addr,'') || ' ' || coalesce(snippet,'')))
) STORED;
CREATE INDEX messages_busqueda_idx ON messages USING gin (busqueda);
CREATE INDEX messages_from_plegado_trgm_idx ON messages
    USING gin (plegar(from_addr) gin_trgm_ops) WHERE deleted_at IS NULL;
CREATE INDEX messages_subject_plegado_trgm_idx ON messages
    USING gin (plegar(subject) gin_trgm_ops) WHERE deleted_at IS NULL;
DROP INDEX IF EXISTS messages_fts_idx;
DROP INDEX IF EXISTS messages_from_trgm_idx;
DROP INDEX IF EXISTS messages_subject_trgm_idx;
