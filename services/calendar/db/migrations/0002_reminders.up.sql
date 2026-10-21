-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Cola de recordatorios (VALARM): el store precalcula, al crear/editar un evento,
-- el instante absoluto de cada disparo (trigger_at) a partir de sus VALARM. El
-- worker de avisos la drena por (trigger_at) y publica `calendar.reminder.due`
-- en el outbox. Precalcular + indexar evita escanear y re-parsear iCalendar en
-- cada tick (escala: sin seq-scans; índice para el WHERE/ORDER BY del worker).

CREATE TABLE calendar_reminders (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    -- destinatario del aviso: el dueño del calendario (claim sub de Sytue ID).
    owner_sub       text        NOT NULL,
    calendar_id     uuid        NOT NULL REFERENCES calendars (id) ON DELETE CASCADE,
    -- object_id agrupa los recordatorios de un recurso; el store los reemplaza en
    -- bloque al reescribir el evento (borra por object_id + reinserta).
    object_id       uuid        NOT NULL REFERENCES calendar_objects (id) ON DELETE CASCADE,
    event_id        uuid        NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    uid             text        NOT NULL,
    summary         text        NOT NULL DEFAULT '',
    -- dtstart del evento (para el payload del aviso).
    dtstart         timestamptz NOT NULL,
    -- instante absoluto (UTC) en que debe dispararse el aviso.
    trigger_at      timestamptz NOT NULL,
    -- ACTION del VALARM (DISPLAY | EMAIL | AUDIO); informativo para el consumidor.
    action          text        NOT NULL DEFAULT 'DISPLAY',
    -- fired_at NULL = pendiente; el worker lo fija al publicar (idempotencia).
    fired_at        timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- Cola del worker: los pendientes ordenados por disparo. Índice parcial (solo
-- pendientes) para que el escaneo no toque los ya disparados (crecimiento acotado
-- por la purga de abajo).
CREATE INDEX calendar_reminders_due_idx
    ON calendar_reminders (trigger_at)
    WHERE fired_at IS NULL;

-- Reemplazo en bloque por objeto (camino de escritura del evento).
CREATE INDEX calendar_reminders_object_idx
    ON calendar_reminders (tenant, object_id);

-- ---------------------------------------------------------------------------
-- RLS: aislamiento por tenant + política PERMISSIVE de mantenimiento para que el
-- worker (cross-tenant, sin `sub` de usuario) drene la cola con `app.maintenance`
-- fijado (InMaintenanceTx), sin necesidad de un rol BYPASSRLS. El camino de
-- escritura (InTenantTx) usa la política por tenant.
-- ---------------------------------------------------------------------------
ALTER TABLE calendar_reminders ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_reminders FORCE ROW LEVEL SECURITY;

CREATE POLICY calendar_reminders_tenant_isolation ON calendar_reminders
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY calendar_reminders_maintenance ON calendar_reminders
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
