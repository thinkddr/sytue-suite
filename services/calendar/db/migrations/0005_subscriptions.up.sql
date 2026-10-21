-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Suscripción a calendarios ICS externos (el «Añadir calendario por URL» de
-- Google Calendar): una suscripción es un CALENDARIO de SOLO LECTURA cuyos
-- VEVENT se obtienen periódicamente de una URL http(s) y se cachean como
-- objetos/eventos normales — así el render, el color, el servido CalDAV, la
-- expansión de RRULE y el TZ/DST reutilizan el camino existente sin duplicarlo.
--
-- read_only marca el calendario como no editable (el store rechaza escrituras de
-- usuario; el worker de refresco escribe por un camino aparte). El estado de
-- sincronización (URL, validadores condicionales ETag/Last-Modified, cadencia)
-- vive en calendar_subscriptions (1:1 con el calendario).

-- Marca de solo-lectura del calendario (suscripciones ICS). Los calendarios
-- normales siguen siendo editables (default false).
ALTER TABLE calendars
    ADD COLUMN read_only boolean NOT NULL DEFAULT false;

-- ---------------------------------------------------------------------------
-- Suscripciones ICS externas. next_refresh_at es la COLA del worker: se reclama
-- por (next_refresh_at) con FOR UPDATE SKIP LOCKED y se «arrienda» empujando
-- next_refresh_at hacia delante al reclamar, de modo que varias réplicas del
-- worker no refrescan la misma suscripción a la vez (cross-réplica seguro, como
-- la cola de recordatorios). etag/last_modified guardan los validadores del
-- último 200 para el GET condicional (If-None-Match / If-Modified-Since): si el
-- feed no cambió (304) no se retrae el contenido.
-- ---------------------------------------------------------------------------
CREATE TABLE calendar_subscriptions (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant                   text        NOT NULL,
    -- 1:1 con el calendario de solo-lectura que respalda la suscripción; al
    -- borrar el calendario (quitar la suscripción) cae en cascada.
    calendar_id              uuid        NOT NULL UNIQUE REFERENCES calendars (id) ON DELETE CASCADE,
    -- url del feed ICS (http/https; validada SSRF en cada fetch).
    url                      text        NOT NULL,
    -- cadencia de refresco del feed (segundos). El worker respeta además el
    -- Cache-Control: max-age de la respuesta si es mayor.
    refresh_interval_seconds integer     NOT NULL DEFAULT 3600,
    -- validadores condicionales del último 200 (RFC 9110 §8.8): permiten el GET
    -- condicional que evita retraer el feed si no ha cambiado.
    etag                     text        NOT NULL DEFAULT '',
    last_modified            text        NOT NULL DEFAULT '',
    last_fetched_at          timestamptz,
    -- next_refresh_at: instante en que toca refrescar (cola del worker). Nuevas
    -- suscripciones nacen con now() para el primer fetch inmediato.
    next_refresh_at          timestamptz NOT NULL DEFAULT now(),
    -- last_status: '' | 'ok' | 'not_modified' | 'error' (informativo para la UI).
    last_status              text        NOT NULL DEFAULT '',
    last_error               text        NOT NULL DEFAULT '',
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT calsub_url_not_blank CHECK (length(url) > 0),
    CONSTRAINT calsub_interval_positive CHECK (refresh_interval_seconds > 0)
);

-- Cola del worker: pendientes ordenados por su próximo refresco.
CREATE INDEX calendar_subscriptions_due_idx
    ON calendar_subscriptions (next_refresh_at);

-- ---------------------------------------------------------------------------
-- RLS: aislamiento por tenant (crear/listar/refrescar-ahora/marcar/sincronizar
-- van tenant-scoped) + política PERMISSIVE de mantenimiento para que el worker
-- (cross-tenant, sin `sub` de usuario) reclame la cola con `app.maintenance`
-- fijado (InMaintenanceTx). Mismo patrón que calendar_reminders.
-- ---------------------------------------------------------------------------
ALTER TABLE calendar_subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_subscriptions FORCE ROW LEVEL SECURITY;

CREATE POLICY calendar_subscriptions_tenant_isolation ON calendar_subscriptions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY calendar_subscriptions_maintenance ON calendar_subscriptions
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
