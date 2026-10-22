-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Lo social de Sytue Cast: los comentarios bajo un vídeo y el chat de un directo.
--
-- Las dos tablas cuelgan del vídeo y viven en SU organización (`tenant` es el
-- del vídeo, no el de quien escribe): una persona invitada de otra organización
-- comenta en el tenant del vídeo, igual que los externos de Rooms (ADR 0040).
-- RLS deny-by-default por `app.tenant`, y en la misma migración la política de
-- fusión literal que exige `platform/pkg/migcheck`.

-- ---------------------------------------------------------------------------
-- Comentarios: hilos de UN nivel (`reply_to` apunta siempre a un comentario raíz;
-- el servicio aplana lo que llegue más hondo). Borrado blando: un comentario con
-- respuestas vivas se queda como hueco para que el hilo no pierda el sentido.
-- ---------------------------------------------------------------------------
CREATE TABLE comments (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    video_id        uuid        NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    author_sub      text        NOT NULL,
    author_name     text        NOT NULL DEFAULT '',
    author_username text        NOT NULL DEFAULT '',
    body            text        NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
    reply_to        uuid        REFERENCES comments (id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);

CREATE INDEX comments_por_video ON comments (video_id, created_at) WHERE deleted_at IS NULL;
CREATE INDEX comments_respuestas ON comments (reply_to) WHERE reply_to IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Chat del directo. Sin hilos, sin edición; se retira a los 30 días (limpieza en
-- mantenimiento, `social.Limpiar`).
-- ---------------------------------------------------------------------------
CREATE TABLE chat_messages (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    video_id        uuid        NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    author_sub      text        NOT NULL,
    author_name     text        NOT NULL DEFAULT '',
    body            text        NOT NULL CHECK (length(body) BETWEEN 1 AND 500),
    created_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);

CREATE INDEX chat_por_video ON chat_messages (video_id, created_at);
-- La limpieza por antigüedad recorre todas las organizaciones a la vez.
CREATE INDEX chat_por_antiguedad ON chat_messages (created_at);

-- ---------------------------------------------------------------------------
-- Aislamiento por organización.
-- ---------------------------------------------------------------------------
ALTER TABLE comments      ENABLE ROW LEVEL SECURITY;
ALTER TABLE comments      FORCE ROW LEVEL SECURITY;
ALTER TABLE chat_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE chat_messages FORCE ROW LEVEL SECURITY;

CREATE POLICY comments_tenant_isolation ON comments
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY chat_messages_tenant_isolation ON chat_messages
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY comments_platform_merge ON comments
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY chat_messages_platform_merge ON chat_messages
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- Lectura pública de los comentarios, SIN sesión y cruzando organizaciones.
--
-- Solo con `app.cast_publico='on'` —la fija una única función del store— y solo
-- para comentarios de un vídeo que se pueda ver sin sesión: el EXISTS sobre
-- `videos` corre a su vez bajo la RLS de `videos`, cuya política `videos_publico`
-- responde a la misma variable, así que aquí no se abre nada que allí esté cerrado.
-- Las condiciones se repiten a propósito: si la política de `videos` cambiara, esta
-- seguiría exigiendo lo mismo.
CREATE POLICY comments_publico ON comments
    FOR SELECT
    USING (
        current_setting('app.cast_publico', true) = 'on'
        AND EXISTS (
            SELECT 1 FROM videos v
             WHERE v.id = comments.video_id
               AND v.deleted_at IS NULL
               AND v.visibility IN ('public', 'unlisted')
               AND v.status IN ('ready', 'live')
        )
    );

-- «Exportar tus datos»: quien escribió un comentario lo lee aunque el vídeo sea de
-- otra organización. Solo SUS filas, con `app.cast_persona` = su sub (la fija una
-- única función del store de lo social).
CREATE POLICY comments_autor ON comments
    FOR SELECT
    USING (author_sub = current_setting('app.cast_persona', true));

-- Retención del chat: la limpieza borra por antigüedad en una transacción de
-- mantenimiento, que cruza organizaciones sin BYPASSRLS. Lectura ADEMÁS de
-- borrado: un DELETE que lee columnas (`WHERE created_at < …`) pasa también por
-- las políticas de SELECT, y sin esta la limpieza no vería nada que borrar.
CREATE POLICY chat_messages_mantenimiento_lectura ON chat_messages
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');

CREATE POLICY chat_messages_mantenimiento ON chat_messages
    FOR DELETE
    USING (current_setting('app.maintenance', true) = 'on');
