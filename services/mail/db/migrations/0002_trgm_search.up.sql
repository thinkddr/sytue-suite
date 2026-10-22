-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Búsqueda por subcadena de remitente/asunto: SearchMessages usa
-- `from_addr ILIKE '%x%'` y `subject ILIKE '%x%'`, que sin índice fuerzan un
-- seq-scan sobre messages. Índices GIN trigram (pg_trgm, contrib estándar de
-- PostgreSQL, licencia permisiva) para servir esos ILIKE. Parciales sobre
-- deleted_at IS NULL porque la búsqueda siempre excluye borrados.
-- El texto libre ya usa FTS (messages_fts_idx); no se toca.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX messages_from_trgm_idx ON messages
    USING gin (from_addr gin_trgm_ops) WHERE deleted_at IS NULL;

CREATE INDEX messages_subject_trgm_idx ON messages
    USING gin (subject gin_trgm_ops) WHERE deleted_at IS NULL;
