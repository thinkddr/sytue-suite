-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Esquema base de Sytue Chat: equipos (teams) por tenant, canales, miembros,
-- mensajes (con hilos vía parent_id), reacciones, menciones, adjuntos de Drive,
-- estado de lectura y outbox de eventos. PostgreSQL es la única fuente de verdad;
-- el fan-out en tiempo real se hace por LISTEN/NOTIFY (sin Redis/Valkey).
--
-- Multi-tenant: cada fila lleva `tenant` (slug). RLS deny-by-default aísla por
-- la variable de sesión `app.tenant` (la fija pg.SetTenant en cada transacción).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Teams: espacios de trabajo de mensajería dentro de un tenant.
-- ---------------------------------------------------------------------------
CREATE TABLE teams (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    slug        text        NOT NULL,
    name        text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    created_by  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz,
    CONSTRAINT teams_name_not_blank CHECK (length(name) > 0)
);

CREATE UNIQUE INDEX teams_unique_slug ON teams (tenant, slug) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Team members: pertenencia de un usuario (sub de Sytue ID) a un team.
-- ---------------------------------------------------------------------------
CREATE TABLE team_members (
    tenant     text        NOT NULL,
    team_id    uuid        NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    subject    text        NOT NULL,
    name       text        NOT NULL DEFAULT '',
    role       text        NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    joined_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, team_id, subject)
);

CREATE INDEX team_members_by_subject ON team_members (tenant, subject);

-- ---------------------------------------------------------------------------
-- Channels: canales públicos/privados/directos dentro de un team.
-- ---------------------------------------------------------------------------
CREATE TABLE channels (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant           text        NOT NULL,
    team_id          uuid        NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    slug             text        NOT NULL,
    name             text        NOT NULL,
    topic            text        NOT NULL DEFAULT '',
    kind             text        NOT NULL DEFAULT 'public' CHECK (kind IN ('public', 'private', 'direct')),
    archived         boolean     NOT NULL DEFAULT false,
    created_by       text        NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    last_message_at  timestamptz,
    deleted_at       timestamptz,
    CONSTRAINT channels_name_not_blank CHECK (length(name) > 0)
);

CREATE UNIQUE INDEX channels_unique_slug
    ON channels (tenant, team_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX channels_by_team ON channels (tenant, team_id) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Channel members: para canales privados/directos y estado de pertenencia.
-- Los canales públicos son legibles por cualquier miembro del team, pero la
-- pertenencia explícita habilita notificaciones y estado de lectura.
-- ---------------------------------------------------------------------------
CREATE TABLE channel_members (
    tenant     text        NOT NULL,
    channel_id uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    subject    text        NOT NULL,
    name       text        NOT NULL DEFAULT '',
    role       text        NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    joined_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, channel_id, subject)
);

CREATE INDEX channel_members_by_subject ON channel_members (tenant, subject);

-- ---------------------------------------------------------------------------
-- Messages: mensajes de canal. parent_id no nulo => respuesta en hilo cuya raíz
-- es parent_id. mentions: subs mencionados. attachments: ficheros de Drive.
-- ---------------------------------------------------------------------------
CREATE TABLE messages (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    channel_id  uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    parent_id   uuid        REFERENCES messages (id) ON DELETE CASCADE,
    author_sub  text        NOT NULL,
    author_name text        NOT NULL DEFAULT '',
    body        text        NOT NULL,
    mentions    text[]      NOT NULL DEFAULT '{}',
    attachments jsonb       NOT NULL DEFAULT '[]'::jsonb,
    reply_count integer     NOT NULL DEFAULT 0,
    edited_at   timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz,
    CONSTRAINT messages_body_not_blank CHECK (length(body) > 0)
);

-- Paginación keyset por canal (created_at, id) DESC; sin N+1 (CONVENTIONS §8).
CREATE INDEX messages_channel_feed
    ON messages (tenant, channel_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL AND parent_id IS NULL;
-- Respuestas de un hilo.
CREATE INDEX messages_thread_feed
    ON messages (tenant, parent_id, created_at)
    WHERE deleted_at IS NULL AND parent_id IS NOT NULL;
-- Menciones a un usuario (notificaciones).
CREATE INDEX messages_mentions_gin ON messages USING gin (mentions);

-- ---------------------------------------------------------------------------
-- Reactions: una fila por (mensaje, usuario, emoji). Se agregan al leer.
-- ---------------------------------------------------------------------------
CREATE TABLE reactions (
    tenant     text        NOT NULL,
    message_id uuid        NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    subject    text        NOT NULL,
    emoji      text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, message_id, subject, emoji)
);

CREATE INDEX reactions_by_message ON reactions (tenant, message_id);

-- ---------------------------------------------------------------------------
-- Read state: hasta dónde ha leído cada usuario cada canal (badge de no leídos).
-- ---------------------------------------------------------------------------
CREATE TABLE read_state (
    tenant                text        NOT NULL,
    channel_id            uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    subject               text        NOT NULL,
    last_read_at          timestamptz NOT NULL DEFAULT now(),
    last_read_message_id  uuid,
    PRIMARY KEY (tenant, channel_id, subject)
);

-- ---------------------------------------------------------------------------
-- Outbox de eventos (sobre PostgreSQL; sin Redis/Valkey). DDL compartido de
-- platform/pkg/events, replicado aquí porque cada servicio gestiona su esquema.
-- El relay LISTEN/NOTIFY hace el fan-out en tiempo real a los clientes WS.
-- ---------------------------------------------------------------------------
CREATE TABLE events_outbox (
    id           uuid        PRIMARY KEY,
    channel      text        NOT NULL,
    type         text        NOT NULL,
    tenant       text        NOT NULL DEFAULT '',
    payload      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    attempts     int         NOT NULL DEFAULT 0,
    last_error   text
);

CREATE INDEX events_outbox_unprocessed_idx
    ON events_outbox (created_at)
    WHERE processed_at IS NULL;

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por tenant deny-by-default (CONVENTIONS §10).
-- Las políticas comparan `tenant` con current_setting('app.tenant', true), que
-- fija pg.SetTenant LOCAL a cada transacción de negocio.
-- ---------------------------------------------------------------------------
ALTER TABLE teams           ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members    ENABLE ROW LEVEL SECURITY;
ALTER TABLE channels        ENABLE ROW LEVEL SECURITY;
ALTER TABLE channel_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages        ENABLE ROW LEVEL SECURITY;
ALTER TABLE reactions       ENABLE ROW LEVEL SECURITY;
ALTER TABLE read_state      ENABLE ROW LEVEL SECURITY;

ALTER TABLE teams           FORCE ROW LEVEL SECURITY;
ALTER TABLE team_members    FORCE ROW LEVEL SECURITY;
ALTER TABLE channels        FORCE ROW LEVEL SECURITY;
ALTER TABLE channel_members FORCE ROW LEVEL SECURITY;
ALTER TABLE messages        FORCE ROW LEVEL SECURITY;
ALTER TABLE reactions       FORCE ROW LEVEL SECURITY;
ALTER TABLE read_state      FORCE ROW LEVEL SECURITY;

CREATE POLICY teams_tenant_isolation ON teams
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY team_members_tenant_isolation ON team_members
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY channels_tenant_isolation ON channels
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY channel_members_tenant_isolation ON channel_members
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY messages_tenant_isolation ON messages
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY reactions_tenant_isolation ON reactions
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY read_state_tenant_isolation ON read_state
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
