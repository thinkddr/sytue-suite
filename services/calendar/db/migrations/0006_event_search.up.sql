-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Búsqueda server-side de eventos por TEXTO (título/descripción/lugar/organizador
-- + asistentes) con rango de fechas y filtro por calendario opcionales. Igual que
-- Contacts/Drive/Mail: pg_trgm (contrib estándar de PostgreSQL, licencia
-- permisiva) con índice GIN de trigramas que sirve el `ILIKE '%q%'` a ESCALA — un
-- patrón con comodín inicial NO lo puede servir un btree (forzaría seq-scan de
-- todos los eventos vivos del tenant; CONVENTIONS §0.8: índice para todo WHERE de
-- texto).
--
-- Columna GENERATED STORED (no un segundo trigger): se autocalcula para las filas
-- existentes y las nuevas. Se concatena con `||` (no concat_ws, STABLE — una
-- columna generada exige expresión IMMUTABLE); es seguro porque summary,
-- description, location y organizer son NOT NULL DEFAULT '' (nunca propagan NULL).
-- El índice es parcial sobre deleted_at IS NULL: la búsqueda solo mira eventos
-- vivos.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE events
    ADD COLUMN search_text text
    GENERATED ALWAYS AS (
        lower(summary || ' ' || description || ' ' || location || ' ' || organizer)
    ) STORED;

CREATE INDEX events_search_trgm_idx
    ON events USING gin (search_text gin_trgm_ops)
    WHERE deleted_at IS NULL;

-- Asistentes: el término también casa contra el email/nombre del ATTENDEE (un
-- EXISTS en la búsqueda). GIN trgm sobre lower(email || ' ' || common_name) para
-- servir ese `ILIKE '%q%'` a escala. attendees no tiene deleted_at (CASCADE de
-- events); la búsqueda ya acota por el evento vivo del EXISTS.
CREATE INDEX attendees_search_trgm_idx
    ON attendees USING gin (lower(email || ' ' || common_name) gin_trgm_ops);

-- Keyset (dtstart, id) de la paginación de resultados (orden estable, sin OFFSET):
-- tras el bitmap de trigramas, sirve el ORDER BY (dtstart, id) y el predicado
-- keyset. Solo maestros (recurrence_id IS NULL): una fila por serie/evento, no una
-- por ocurrencia.
CREATE INDEX events_search_order_idx
    ON events (tenant, dtstart, id)
    WHERE deleted_at IS NULL AND recurrence_id IS NULL;
