-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Videollamada de Sytue Rooms: el esquema que hasta ahora vivía en la base de datos
-- aparte del servicio de vídeo, traído a la única base de datos de Rooms.
--
-- Mensajería y videollamada eran dos servicios con dos bases: para publicar la tarjeta
-- de «llamada finalizada» en un canal, el vídeo tenía que llamar por HTTP a la
-- mensajería con un secreto compartido. Al ser un solo servicio, esa conversación es
-- una llamada en proceso y una transacción — y el secreto que había que cablear en dos
-- sitios (y que si faltaba dejaba la verja de acceso al canal fallando cerrada, con un
-- 403 inexplicable al abrir una llamada) deja de existir.
--
-- El `events_outbox` NO se recrea: la mensajería ya lo tiene en esta misma base, y el
-- outbox es del servicio, no del producto. Los eventos del vídeo se publican en él.
--
-- Las tablas conservan sus nombres (`rooms` = salas de vídeo). Renombrarlas sería
-- cambiar el modelo, no fusionar los servicios.

-- ---------------------------------------------------------------------------
-- Salas de vídeo: cada una orquesta una room de LiveKit. `name` es el
-- identificador técnico (room name en LiveKit), único por tenant.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS rooms (
    id                    uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant                text        NOT NULL,
    name                  text        NOT NULL,
    title                 text        NOT NULL,
    status                text        NOT NULL DEFAULT 'scheduled'
                                      CHECK (status IN ('scheduled', 'active', 'ended')),
    created_by            text        NOT NULL DEFAULT '',
    max_participants      integer     NOT NULL DEFAULT 0 CHECK (max_participants >= 0),
    empty_timeout_seconds integer     NOT NULL DEFAULT 0 CHECK (empty_timeout_seconds >= 0),
    -- livekit_sid: SID de la room asignado por LiveKit (al iniciarse).
    livekit_sid           text        NOT NULL DEFAULT '',
    -- metadata: JSON opaco (canal de origen, tenant embebido para los webhooks).
    metadata              jsonb       NOT NULL DEFAULT '{}'::jsonb,
    started_at            timestamptz,
    ended_at              timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT rooms_title_not_blank CHECK (length(title) > 0)
);

-- Nombre técnico único POR TENANT (no global: los webhooks portan además el SID).
CREATE UNIQUE INDEX IF NOT EXISTS rooms_unique_name_per_tenant ON rooms (tenant, name);
CREATE INDEX IF NOT EXISTS rooms_tenant_status_idx ON rooms (tenant, status, created_at DESC);

-- ---------------------------------------------------------------------------
-- Participantes: presencia derivada de los webhooks participant_joined/left de
-- LiveKit, conservada como historial. `livekit_sid` identifica una conexión.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS participants (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    room_id     uuid        NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    -- identity: identidad en LiveKit (= sub de Sytue ID).
    identity    text        NOT NULL,
    name        text        NOT NULL DEFAULT '',
    role        text        NOT NULL DEFAULT 'speaker'
                            CHECK (role IN ('host', 'speaker', 'viewer')),
    livekit_sid text        NOT NULL DEFAULT '',
    joined_at   timestamptz NOT NULL DEFAULT now(),
    left_at     timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS participants_unique_sid ON participants (tenant, room_id, livekit_sid);
CREATE INDEX IF NOT EXISTS participants_room_idx ON participants (tenant, room_id, joined_at);
CREATE INDEX IF NOT EXISTS participants_present_idx ON participants (tenant, room_id) WHERE left_at IS NULL;

-- ---------------------------------------------------------------------------
-- Metadatos de grabación (egress de LiveKit). El fichero vive en Object Storage;
-- aquí solo el metadato y su estado.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS recordings_meta (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    room_id      uuid        NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    egress_id    text        NOT NULL,
    status       text        NOT NULL DEFAULT 'started'
                             CHECK (status IN ('started', 'completed', 'failed')),
    storage_key  text        NOT NULL DEFAULT '',
    size_bytes   bigint      NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    duration_sec bigint      NOT NULL DEFAULT 0 CHECK (duration_sec >= 0),
    started_at   timestamptz NOT NULL DEFAULT now(),
    ended_at     timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS recordings_unique_egress ON recordings_meta (tenant, egress_id);
CREATE INDEX IF NOT EXISTS recordings_room_idx ON recordings_meta (tenant, room_id, started_at DESC);

-- ---------------------------------------------------------------------------
-- Resolución acotada del tenant por nombre de sala. Los webhooks de egress de
-- LiveKit no portan tenant ni metadata de room; esta función SECURITY DEFINER
-- devuelve SOLO el slug del tenant de una sala existente, sin exponer ninguna otra
-- fila, y ninguna si no existe (el store la mapea a «no encontrado»).
-- ---------------------------------------------------------------------------
DROP FUNCTION IF EXISTS meet_resolve_room_tenant(text);
CREATE FUNCTION meet_resolve_room_tenant(p_name text)
RETURNS SETOF text
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
    SELECT tenant FROM public.rooms WHERE name = p_name LIMIT 1;
$$;

-- ---------------------------------------------------------------------------
-- Row-Level Security: aislamiento por tenant deny-by-default (CONVENTIONS §10).
-- ---------------------------------------------------------------------------
ALTER TABLE rooms           ENABLE ROW LEVEL SECURITY;
ALTER TABLE participants    ENABLE ROW LEVEL SECURITY;
ALTER TABLE recordings_meta ENABLE ROW LEVEL SECURITY;

ALTER TABLE rooms           FORCE ROW LEVEL SECURITY;
ALTER TABLE participants    FORCE ROW LEVEL SECURITY;
ALTER TABLE recordings_meta FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rooms_tenant_isolation ON rooms;
CREATE POLICY rooms_tenant_isolation ON rooms
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

DROP POLICY IF EXISTS participants_tenant_isolation ON participants;
CREATE POLICY participants_tenant_isolation ON participants
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

DROP POLICY IF EXISTS recordings_tenant_isolation ON recordings_meta;
CREATE POLICY recordings_tenant_isolation ON recordings_meta
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
