-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Búsqueda por SUBCADENA y tolerante a erratas en contactos. La 0001 solo dejó
-- FTS (contacts_search_idx sobre el tsvector `search`): websearch_to_tsquery no
-- casa infijos ("ana" no encuentra "Mariana") ni erratas. Se añade pg_trgm
-- (contrib estándar de PostgreSQL, licencia permisiva) sobre una columna
-- generada con los campos legibles del contacto, e índice GIN de trigramas que
-- sirve el ILIKE '%q%' y el ranking por similarity() de SearchContacts.
--
-- Columna GENERATED STORED (no un segundo trigger): se autocalcula para las
-- filas existentes y las nuevas. Se concatena con `||` (no concat_ws, que es
-- STABLE y una columna generada exige expresión IMMUTABLE); es seguro porque
-- estas columnas son NOT NULL DEFAULT '' (nunca propagan NULL). El índice es
-- parcial sobre deleted_at IS NULL porque la búsqueda excluye borrados. La nota
-- se deja fuera: puede ser larga y engordaría el índice sin aportar a la
-- búsqueda por nombre/organización.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE contacts
    ADD COLUMN search_text text
    GENERATED ALWAYS AS (
        lower(
            formatted_name || ' ' || family_name || ' ' || given_name || ' ' ||
            nickname || ' ' || organization || ' ' || title
        )
    ) STORED;

CREATE INDEX contacts_search_trgm_idx
    ON contacts USING gin (search_text gin_trgm_ops)
    WHERE deleted_at IS NULL;
