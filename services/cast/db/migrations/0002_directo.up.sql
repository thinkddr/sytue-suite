-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El directo de Sytue Cast (CAST-ENCARGO §4 0002, §6): la clave de emisión de
-- cada persona y quién está mirando cada directo.
--
-- Las dos tablas viven en la organización (`tenant`) con RLS deny-by-default por
-- `app.tenant` y la política de fusión literal que exige `platform/pkg/migcheck`.
-- Hay UNA lectura que cruza organizaciones: cuando LiveKit avisa de que un
-- ingress ha empezado a emitir solo trae su `ingress_id`, y con él hay que saber
-- de qué organización y de quién es la clave. Esa lectura va con
-- `app.cast_ingest='on'`, que fija una sola función del store del directo.

-- ---------------------------------------------------------------------------
-- Clave de emisión (la que se pega en OBS). Una por persona y organización, y
-- detrás de cada una un ingress de LiveKit. NO lleva room: el room es del
-- vídeo (`videos.live_room`, `cast-<tenant>-<video_id>`) y el ingress se
-- apunta al del directo que se programa; si OBS emite sin programar nada, el
-- servicio crea el directo sobre el room al que el ingress apuntaba.
--
-- La clave se guarda tal cual porque LiveKit la genera y no la vuelve a enseñar
-- más que listando ingresos; es la única forma de volver a dársela a su dueño.
-- ---------------------------------------------------------------------------
CREATE TABLE stream_keys (
    tenant      text        NOT NULL,
    owner_sub   text        NOT NULL,
    key         text        NOT NULL UNIQUE,
    ingress_id  text        NOT NULL UNIQUE,
    -- Cómo aparece el emisor en la room; se enseña en el aviso «está en directo».
    owner_name  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    rotated_at  timestamptz,
    PRIMARY KEY (tenant, owner_sub)
);

-- ---------------------------------------------------------------------------
-- Quién está viendo un directo ahora, por identidad de LiveKit (el sub de la
-- persona o `anon-<hex>`). De aquí salen `viewers_now` y `viewers_peak` del
-- vídeo; se vacía al terminar. Ni el emisor ni el egress entran.
-- ---------------------------------------------------------------------------
CREATE TABLE live_viewers (
    video_id   uuid        NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    tenant     text        NOT NULL,
    identity   text        NOT NULL,
    joined_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (video_id, identity)
);

-- ---------------------------------------------------------------------------
-- Aislamiento por organización.
-- ---------------------------------------------------------------------------
ALTER TABLE stream_keys  ENABLE ROW LEVEL SECURITY;
ALTER TABLE stream_keys  FORCE ROW LEVEL SECURITY;
ALTER TABLE live_viewers ENABLE ROW LEVEL SECURITY;
ALTER TABLE live_viewers FORCE ROW LEVEL SECURITY;

CREATE POLICY stream_keys_tenant_isolation ON stream_keys
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY live_viewers_tenant_isolation ON live_viewers
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY stream_keys_platform_merge ON stream_keys
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY live_viewers_platform_merge ON live_viewers
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- El webhook y el sondeo resuelven de quién es un ingress solo con su id, antes
-- de saber la organización: solo lectura, y solo con esta variable, que fija
-- una única función del store del directo (`enIngesta`).
CREATE POLICY stream_keys_ingesta ON stream_keys
    FOR SELECT
    USING (current_setting('app.cast_ingest', true) = 'on');
