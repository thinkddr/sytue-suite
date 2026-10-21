-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Escala: índice trigram para la búsqueda de mensajes (ILIKE '%q%') e índice
-- para la purga periódica del outbox por antigüedad.

-- pg_trgm es contrib estándar de PostgreSQL (licencia permisiva).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- SearchMessages filtra `body ILIKE '%q%'` sobre mensajes vivos: sin índice es
-- un seq scan de toda la tabla. GIN trigram parcial (solo mensajes no borrados,
-- igual que el WHERE real de la consulta).
CREATE INDEX messages_body_trgm
    ON messages USING gin (body gin_trgm_ops)
    WHERE deleted_at IS NULL;

-- La purga del outbox borra por `created_at < now() - interval '48 hours'` sin
-- filtrar por processed_at; el índice parcial existente (processed_at IS NULL)
-- no es aplicable a ese WHERE.
CREATE INDEX events_outbox_created_idx ON events_outbox (created_at);
