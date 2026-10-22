-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base de Sytue Hub (backend de integración seamless de la suite):
--   - notifications : notificaciones unificadas por usuario (durables).
--   - presence      : estado de presencia efímero por usuario.
--
-- La búsqueda federada y los intents cross-app NO persisten estado en Hub
-- (consultan los servicios de la suite en tiempo de petición y componen
-- deep-links), por lo que no añaden tablas. PostgreSQL es la única fuente de
-- verdad; el fan-out de notificaciones en tiempo real se hace por LISTEN/NOTIFY
-- (canal sytue_hub_notifications), sin Redis/Valkey.
--
-- Multi-tenant: cada fila lleva `tenant` (slug). RLS deny-by-default aísla por
-- la variable de sesión `app.tenant` (la fija pg.SetTenant en cada transacción).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Notifications: avisos unificados del shell, por usuario (sub de Sytue ID).
-- ---------------------------------------------------------------------------
CREATE TABLE notifications (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant     text        NOT NULL,
    subject    text        NOT NULL,
    source     text        NOT NULL CHECK (source IN ('drive', 'mail', 'calendar', 'contacts', 'chat')),
    type       text        NOT NULL,
    level      text        NOT NULL DEFAULT 'info' CHECK (level IN ('info', 'success', 'warning', 'error')),
    title      text        NOT NULL,
    body       text        NOT NULL DEFAULT '',
    url        text        NOT NULL DEFAULT '',
    read       boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at    timestamptz,
    CONSTRAINT notifications_title_not_blank CHECK (length(title) > 0)
);

-- Feed por usuario con paginación keyset (created_at, id) DESC; sin N+1.
CREATE INDEX notifications_user_feed
    ON notifications (tenant, subject, created_at DESC, id DESC);
-- Conteo de no leídas y filtrado por estado (índice parcial compacto).
CREATE INDEX notifications_unread
    ON notifications (tenant, subject)
    WHERE read = false;
-- Soporte a la purga por retención (notificaciones leídas antiguas).
CREATE INDEX notifications_read_at
    ON notifications (read_at)
    WHERE read = true;

-- ---------------------------------------------------------------------------
-- Presence: estado de presencia por usuario (efímero, una fila por usuario).
-- last_seen se renueva en cada heartbeat; la obsolescencia (offline) se deriva
-- al leer comparando con la TTL de presencia.
-- ---------------------------------------------------------------------------
CREATE TABLE presence (
    tenant    text        NOT NULL,
    subject   text        NOT NULL,
    state     text        NOT NULL DEFAULT 'online' CHECK (state IN ('online', 'away', 'busy', 'offline')),
    status    text        NOT NULL DEFAULT '',
    last_seen timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, subject)
);

-- Soporte a la purga de presencia obsoleta.
CREATE INDEX presence_last_seen ON presence (last_seen);

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por tenant deny-by-default (CONVENTIONS §10).
-- Las políticas comparan `tenant` con current_setting('app.tenant', true), que
-- fija pg.SetTenant LOCAL a cada transacción de negocio. Las tareas de
-- mantenimiento (purga) corren con un rol propietario fuera de RLS.
-- ---------------------------------------------------------------------------
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE presence      ENABLE ROW LEVEL SECURITY;

ALTER TABLE notifications FORCE ROW LEVEL SECURITY;
ALTER TABLE presence      FORCE ROW LEVEL SECURITY;

CREATE POLICY notifications_tenant_isolation ON notifications
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY presence_tenant_isolation ON presence
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Política de mantenimiento (PERMISSIVE, se suma a la anterior): habilita las
-- tareas cross-tenant de purga (retención de notificaciones, presencia obsoleta)
-- SOLO cuando la sesión declara explícitamente `app.maintenance = 'on'`. Las
-- transacciones de negocio nunca fijan ese flag, por lo que su aislamiento por
-- tenant no se ve afectado. La fija pg.SetMaintenance en el job de purga.
CREATE POLICY notifications_maintenance ON notifications
    USING (current_setting('app.maintenance', true) = 'on');

CREATE POLICY presence_maintenance ON presence
    USING (current_setting('app.maintenance', true) = 'on');
