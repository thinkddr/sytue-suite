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

-- El cuerpo de los mensajes, YA PLEGADO en una columna guardada. Medido (200 000
-- mensajes, rol de aplicación con RLS forzada): `body ILIKE` 205 ms, `plegar(body)
-- LIKE` 440 ms, esta columna con LIKE 60 ms. Con RLS, Postgres no usa índices de
-- trigramas para el rol de aplicación (LIKE no es LEAKPROOF), así que lo que se
-- ahorra es el plegado de cada fila en cada búsqueda. El índice se queda para
-- quien consulta sin RLS (mantenimiento, eDiscovery como dueño).
-- Autor y canal (from:/in:) usan plegar() al vuelo: son cadenas cortas.
ALTER TABLE messages ADD COLUMN body_plegado text GENERATED ALWAYS AS (plegar(body)) STORED;
CREATE INDEX messages_body_plegado_trgm
    ON messages USING gin (body_plegado gin_trgm_ops)
    WHERE deleted_at IS NULL;
DROP INDEX IF EXISTS messages_body_trgm;
