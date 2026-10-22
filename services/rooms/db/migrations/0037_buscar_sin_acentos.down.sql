-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

CREATE INDEX IF NOT EXISTS messages_body_trgm
    ON messages USING gin (body gin_trgm_ops)
    WHERE deleted_at IS NULL;
DROP INDEX IF EXISTS messages_body_plegado_trgm;
-- sytue:destructive-ok vuelta atrás de 0037: la columna solo la lee la búsqueda nueva
ALTER TABLE messages DROP COLUMN IF EXISTS body_plegado;

-- plegar() y unaccent se quedan: la versión anterior no los usa y retirarlos
-- obligaría a soltar antes cualquier índice que dependa de ellos.
