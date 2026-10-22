-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS attendees_search_plegado_idx;
CREATE INDEX IF NOT EXISTS attendees_search_trgm_idx
    ON attendees USING gin (lower(email || ' ' || common_name) gin_trgm_ops);
ALTER TABLE events ALTER COLUMN search_text
    SET EXPRESSION AS (lower(summary || ' ' || description || ' ' || location || ' ' || organizer));

-- plegar() y unaccent se quedan: la versión anterior no los usa y retirarlos
-- obligaría a soltar antes cualquier índice que dependa de ellos.
