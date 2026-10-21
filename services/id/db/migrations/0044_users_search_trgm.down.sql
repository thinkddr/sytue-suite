-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- La extensión NO se retira: la comparten otros servicios del mismo clúster y
-- quitarla desde aquí les tiraría sus índices de búsqueda.
DROP INDEX IF EXISTS users_search_trgm_idx;
ALTER TABLE users DROP COLUMN IF EXISTS search_text;
