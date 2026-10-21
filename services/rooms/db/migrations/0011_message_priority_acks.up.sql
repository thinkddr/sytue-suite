-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PRIORIDAD de mensaje + ACUSE de recibo (Ola 12 · Profundidad de mensaje).
--
-- priority: 'normal' (por defecto) | 'important' | 'urgent'. El cliente pinta un
-- badge de acento con TEXTO (nunca solo color, WCAG AA).
--
-- requires_ack: el autor pide acuse explícito; cada lector confirma con un clic y
-- se agrega «Confirmado por N» (message_acks), como las reacciones — por consulta
-- agregada en una sola pasada, sin N+1.
--
-- Backfill: ADD COLUMN ... DEFAULT constante lo resuelve Postgres por metadatos,
-- sin reescribir la tabla (barato a escala). messages ya tiene RLS por tenant
-- (0001_init); las columnas nuevas la heredan.

ALTER TABLE messages ADD COLUMN priority text NOT NULL DEFAULT 'normal';
ALTER TABLE messages ADD CONSTRAINT messages_priority_valid
    CHECK (priority IN ('normal', 'important', 'urgent'));
ALTER TABLE messages ADD COLUMN requires_ack boolean NOT NULL DEFAULT false;

-- Acuses de recibo: una fila por (mensaje, lector). Idempotente por PK. El conteo
-- «Confirmado por N» y el estado per-visor se agregan en la lectura de mensajes
-- con una sola consulta ANY($ids) — mismo patrón que reactions, sin N+1. La PK
-- (tenant, message_id, subject) es a la vez el índice que cubre esa agregación
-- (prefijo tenant, message_id).
CREATE TABLE message_acks (
    tenant     text        NOT NULL,
    message_id uuid        NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    subject    text        NOT NULL,
    acked_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, message_id, subject)
);

-- RLS deny-by-default por tenant (mismo patrón que reactions/read_state).
ALTER TABLE message_acks ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_acks FORCE ROW LEVEL SECURITY;
CREATE POLICY message_acks_tenant_isolation ON message_acks
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
