-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

CREATE INDEX IF NOT EXISTS files_name_trgm_idx
    ON files USING gin (lower(name) gin_trgm_ops)
    WHERE deleted_at IS NULL;
DROP INDEX IF EXISTS files_name_plegado_trgm_idx;

-- plegar() y unaccent se quedan: la versión anterior no los usa y retirarlos
-- obligaría a soltar antes cualquier índice que dependa de ellos.
