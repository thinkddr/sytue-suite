-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Tabla outbox compartida para el bus de eventos sobre PostgreSQL
-- (sin Redis/Valkey). Los servicios pueden copiar/incluir esta migración.

CREATE TABLE IF NOT EXISTS events_outbox (
    id           uuid PRIMARY KEY,
    channel      text        NOT NULL,
    type         text        NOT NULL,
    tenant       text        NOT NULL DEFAULT '',
    payload      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    attempts     int         NOT NULL DEFAULT 0,
    last_error   text
);

CREATE INDEX IF NOT EXISTS events_outbox_unprocessed_idx
    ON events_outbox (created_at)
    WHERE processed_at IS NULL;
