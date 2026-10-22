-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS contacts_search_trgm_idx;
ALTER TABLE contacts DROP COLUMN IF EXISTS search_text;
-- pg_trgm puede estar compartida con otros índices/servicios: no se elimina.
