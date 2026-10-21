-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Escala del listado y de la purga del outbox:
--
--  * documents_title_trgm_idx nació como btree sobre (tenant, lower(title)):
--    inútil para la búsqueda por subcadena (ILIKE '%q%') — un btree solo sirve
--    para prefijos. Se sustituye por un índice GIN de trigramas real (pg_trgm),
--    que sí acelera ILIKE con comodín inicial.
--
--  * events_outbox se purga periódicamente por created_at (retención 48h, ver
--    cmd/docs). El índice parcial events_outbox_unprocessed_idx exige
--    processed_at IS NULL en el WHERE y no cubre esa purga -> btree simple.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

DROP INDEX IF EXISTS documents_title_trgm_idx;
CREATE INDEX documents_title_trgm_idx
    ON documents USING gin (title gin_trgm_ops);

CREATE INDEX events_outbox_created_idx ON events_outbox (created_at);
