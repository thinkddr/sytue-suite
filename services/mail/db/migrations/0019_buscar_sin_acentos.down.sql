-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

CREATE INDEX IF NOT EXISTS messages_fts_idx ON messages
    USING gin (to_tsvector('simple', coalesce(subject,'') || ' ' || coalesce(from_addr,'') || ' ' || coalesce(snippet,'')));
CREATE INDEX IF NOT EXISTS messages_from_trgm_idx ON messages
    USING gin (from_addr gin_trgm_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS messages_subject_trgm_idx ON messages
    USING gin (subject gin_trgm_ops) WHERE deleted_at IS NULL;
DROP INDEX IF EXISTS messages_busqueda_idx;
DROP INDEX IF EXISTS messages_from_plegado_trgm_idx;
DROP INDEX IF EXISTS messages_subject_plegado_trgm_idx;
-- sytue:destructive-ok vuelta atrás de 0019: la columna solo la lee la búsqueda nueva
ALTER TABLE messages DROP COLUMN IF EXISTS busqueda;

-- plegar() y unaccent se quedan: la versión anterior no los usa y retirarlos
-- obligaría a soltar antes cualquier índice que dependa de ellos.
