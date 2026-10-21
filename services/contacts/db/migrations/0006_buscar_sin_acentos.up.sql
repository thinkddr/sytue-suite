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

-- La columna de trigramas, plegada; su índice se reconstruye con la tabla.
ALTER TABLE contacts ALTER COLUMN search_text
    SET EXPRESSION AS (plegar(
        formatted_name || ' ' || family_name || ' ' || given_name || ' ' ||
        nickname || ' ' || organization || ' ' || title
    ));

-- El tsvector lo mantiene un trigger: se pliega lo que entra…
CREATE OR REPLACE FUNCTION contacts_search_update() RETURNS trigger AS $$
BEGIN
    NEW.search :=
        setweight(to_tsvector('simple', plegar(coalesce(NEW.formatted_name, ''))), 'A') ||
        setweight(to_tsvector('simple', plegar(coalesce(NEW.family_name, ''))), 'A') ||
        setweight(to_tsvector('simple', plegar(coalesce(NEW.given_name, ''))), 'A') ||
        setweight(to_tsvector('simple', plegar(coalesce(NEW.nickname, ''))), 'B') ||
        setweight(to_tsvector('simple', plegar(coalesce(NEW.organization, ''))), 'B') ||
        setweight(to_tsvector('simple', plegar(coalesce(NEW.title, ''))), 'C') ||
        setweight(to_tsvector('simple', plegar(coalesce(NEW.note, ''))), 'D');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- …y se recalcula lo que ya estaba. Bajo FORCE ROW LEVEL SECURITY un UPDATE sin
-- organización no ve ninguna fila, así que se suelta el FORCE solo dentro de esta
-- transacción (la migración entera es una) y se vuelve a poner antes de terminar.
-- El único trigger de la tabla es el de búsqueda: nada más se entera.
ALTER TABLE contacts NO FORCE ROW LEVEL SECURITY;
UPDATE contacts SET search = NULL;
ALTER TABLE contacts FORCE ROW LEVEL SECURITY;
