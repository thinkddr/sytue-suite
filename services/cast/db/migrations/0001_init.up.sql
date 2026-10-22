-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Cast: los vídeos (subidos o en directo), las personas concretas con las
-- que se comparte uno y la cola de transcodificación (CAST-ENCARGO §4, 0001).
--
-- Cada vídeo vive en la organización de quien lo sube (`tenant`), con RLS
-- deny-by-default por `app.tenant` y la política de fusión literal que exige
-- `platform/pkg/migcheck`. Además hay lecturas que cruzan organizaciones, todas
-- de solo lectura salvo la cola:
--
--   * `app.cast_publico='on'`: sin sesión, los vídeos public y unlisted que ya se
--     pueden ver. La portada y el canal de cada organización leen por aquí.
--   * `app.cast_persona=<sub>`: una persona de CUALQUIER organización ve los
--     vídeos `people` en los que la han puesto, y nada más (ADR 0040: el vídeo
--     vive en la organización de quien lo abre).
--   * `app.maintenance='on'`: el transcodificador toma trabajos de la cola de
--     todas las organizaciones (FOR UPDATE SKIP LOCKED) y escribe el resultado.
--
-- Numeración: 0002 es el directo (claves de emisión, espectadores), 0003 lo social.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Vídeos.
--
-- kind:   upload (la parte «Vimeo») o live (directo desde OBS, navegador o Fyne).
-- status: upload -> uploading -> processing -> ready | failed
--         live   -> scheduled -> live -> ended (-> processing -> ready con la grabación)
-- visibility: public   — portada, canal de la organización, enlace e incrustado
--             unlisted — solo quien tenga el enlace; incrustable
--             org      — la organización del vídeo
--             people   — solo las personas de `video_people` (y quien lo edita)
-- ---------------------------------------------------------------------------
CREATE TABLE videos (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant           text        NOT NULL,
    owner_sub        text        NOT NULL,
    owner_name       text        NOT NULL DEFAULT '',
    kind             text        NOT NULL CHECK (kind IN ('upload', 'live')),
    status           text        NOT NULL CHECK (status IN ('uploading', 'processing', 'ready', 'failed',
                                                            'scheduled', 'live', 'ended')),
    visibility       text        NOT NULL DEFAULT 'org'
                                 CHECK (visibility IN ('public', 'unlisted', 'org', 'people')),
    title            text        NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    description      text        NOT NULL DEFAULT '' CHECK (length(description) <= 5000),
    -- Lo que rellena el transcodificador: medidas y dónde están el original y las
    -- derivadas en el almacén de objetos (claves relativas al bucket de Cast).
    duration_ms      bigint,
    width            integer,
    height           integer,
    original_key     text,
    hls_key          text,
    webm_key         text,
    poster_key       text,
    bytes_original   bigint      NOT NULL DEFAULT 0,
    -- original + derivadas: lo que cuenta en la cuota compartida con Drive (§8).
    bytes_total      bigint      NOT NULL DEFAULT 0,
    comments_enabled boolean     NOT NULL DEFAULT true,
    chat_enabled     boolean     NOT NULL DEFAULT true,
    download_enabled boolean     NOT NULL DEFAULT false,
    -- Directo: el room de LiveKit, por dónde entra la señal y las marcas de tiempo.
    live_room        text,
    live_source      text        CHECK (live_source IN ('rtmp', 'webrtc')),
    live_started_at  timestamptz,
    live_ended_at    timestamptz,
    live_egress_id   text,
    viewers_now      integer     NOT NULL DEFAULT 0,
    viewers_peak     integer     NOT NULL DEFAULT 0,
    views            bigint      NOT NULL DEFAULT 0,
    -- Por qué falló, para que la interfaz lo diga. Vacío si no falló.
    failure          text        NOT NULL DEFAULT '',
    -- La primera vez que se pudo ver (ready o live). La portada ordena por aquí.
    published_at     timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz
);

CREATE INDEX videos_por_dueno ON videos (tenant, owner_sub, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX videos_por_visibilidad ON videos (tenant, visibility, published_at DESC);
-- La portada y los canales: solo lo público y ya visible.
CREATE INDEX videos_portada ON videos (published_at DESC)
    WHERE visibility = 'public' AND deleted_at IS NULL AND status IN ('ready', 'live');

-- ---------------------------------------------------------------------------
-- Personas con las que se comparte un vídeo `people`. De cualquier organización:
-- el `user_id` (sub de Sytue ID) y el nombre los da `people:resolve`, nunca el
-- navegador de quien comparte, porque es como las verá el dueño en la lista.
-- ---------------------------------------------------------------------------
CREATE TABLE video_people (
    video_id  uuid        NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    tenant    text        NOT NULL,
    user_id   text        NOT NULL,
    name      text        NOT NULL DEFAULT '',
    username  text        NOT NULL DEFAULT '',
    added_by  text        NOT NULL,
    added_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (video_id, user_id)
);

-- «¿Qué vídeos me han compartido?» cruza organizaciones por persona.
CREATE INDEX video_people_persona ON video_people (user_id);

-- ---------------------------------------------------------------------------
-- Cola de transcodificación. La escribe el servicio en la organización del vídeo
-- y la consume el transcodificador en mantenimiento, de todas a la vez.
--
-- kind:   upload (un original recién subido), live_vod (la grabación de un
--         directo, desde el HLS del egress), poster (un fotograma elegido).
-- status: pending -> running -> done | failed (tres reintentos; el cuarto deja el
--         vídeo en failed con el motivo).
-- ---------------------------------------------------------------------------
CREATE TABLE transcode_jobs (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    video_id    uuid        NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    kind        text        NOT NULL CHECK (kind IN ('upload', 'live_vod', 'poster')),
    status      text        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'failed')),
    attempts    integer     NOT NULL DEFAULT 0,
    claimed_at  timestamptz,
    claimed_by  text        NOT NULL DEFAULT '',
    error       text        NOT NULL DEFAULT '',
    -- Lo que el trabajo necesita además del vídeo, p. ej. {"at_ms": 12000} para el póster.
    payload     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    -- Cuándo puede volver a intentarse: tras un fallo, con espera creciente, para
    -- que un tropiezo pasajero (S3, red) no agote los intentos en segundos.
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX transcode_jobs_pendientes ON transcode_jobs (next_attempt_at) WHERE status = 'pending';
-- Un solo trabajo activo por vídeo y clase: pedir dos veces el mismo póster o
-- reencolar una subida no duplica el trabajo.
CREATE UNIQUE INDEX transcode_jobs_uno_activo ON transcode_jobs (video_id, kind) WHERE status IN ('pending', 'running');
CREATE INDEX transcode_jobs_por_video ON transcode_jobs (video_id);

-- ---------------------------------------------------------------------------
-- Aislamiento por organización.
-- ---------------------------------------------------------------------------
ALTER TABLE videos         ENABLE ROW LEVEL SECURITY;
ALTER TABLE videos         FORCE ROW LEVEL SECURITY;
ALTER TABLE video_people   ENABLE ROW LEVEL SECURITY;
ALTER TABLE video_people   FORCE ROW LEVEL SECURITY;
ALTER TABLE transcode_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE transcode_jobs FORCE ROW LEVEL SECURITY;

CREATE POLICY videos_tenant_isolation ON videos
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY video_people_tenant_isolation ON video_people
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY transcode_jobs_tenant_isolation ON transcode_jobs
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY videos_platform_merge ON videos
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY video_people_platform_merge ON video_people
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY transcode_jobs_platform_merge ON transcode_jobs
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- Lectura pública, sin sesión y cruzando organizaciones. La fija solo el store
-- (`enPublico`), y la de los comentarios de 0003 repite estas condiciones.
CREATE POLICY videos_publico ON videos
    FOR SELECT
    USING (
        current_setting('app.cast_publico', true) = 'on'
        AND deleted_at IS NULL
        AND visibility IN ('public', 'unlisted')
        AND status IN ('ready', 'live')
    );

-- La persona invitada solo ve SUS filas de `video_people`: le basta para saber
-- qué le han compartido y no enseña con quién más se compartió.
CREATE POLICY video_people_persona ON video_people
    FOR SELECT
    USING (user_id = current_setting('app.cast_persona', true));

-- Y los vídeos `people` donde está. El EXISTS corre bajo la RLS de
-- `video_people`, que con esta misma variable solo deja ver las suyas.
CREATE POLICY videos_persona ON videos
    FOR SELECT
    USING (
        current_setting('app.cast_persona', true) <> ''
        AND deleted_at IS NULL
        AND visibility = 'people'
        AND status IN ('ready', 'live')
        AND EXISTS (
            SELECT 1 FROM video_people p
             WHERE p.video_id = videos.id
               AND p.user_id = current_setting('app.cast_persona', true)
        )
    );

-- El transcodificador: toma trabajos y escribe su resultado en el vídeo, de
-- cualquier organización, en una transacción de mantenimiento sin BYPASSRLS.
-- Lectura ADEMÁS de escritura: un UPDATE que lee columnas pasa también por las
-- políticas de SELECT.
CREATE POLICY transcode_jobs_mantenimiento_lectura ON transcode_jobs
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');

CREATE POLICY transcode_jobs_mantenimiento ON transcode_jobs
    FOR UPDATE
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY videos_mantenimiento_lectura ON videos
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');

CREATE POLICY videos_mantenimiento ON videos
    FOR UPDATE
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
