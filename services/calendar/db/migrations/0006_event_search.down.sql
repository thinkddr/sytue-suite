-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS events_search_order_idx;
DROP INDEX IF EXISTS attendees_search_trgm_idx;
DROP INDEX IF EXISTS events_search_trgm_idx;
ALTER TABLE events DROP COLUMN IF EXISTS search_text;
-- La extensión pg_trgm se deja instalada a propósito: es global y otros objetos
-- podrían depender de ella; su presencia es inocua.
