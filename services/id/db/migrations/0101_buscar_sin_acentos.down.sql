-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE users ALTER COLUMN search_text
    SET EXPRESSION AS (lower(email || ' ' || name));

-- plegar() y unaccent se quedan: la versión anterior no los usa y retirarlos
-- obligaría a soltar antes cualquier índice que dependa de ellos.
