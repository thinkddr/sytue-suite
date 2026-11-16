-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- «TAMBIÉN ENVIAR AL CANAL» (Slack «Also send to #canal»): una respuesta de hilo que
-- su autor quiso que se viera además en el canal. Sigue siendo UNA fila —vive en el
-- hilo, cuenta en reply_count y se edita o borra una sola vez—; la columna solo la
-- deja entrar en el feed del canal, que hasta ahora pintaba únicamente raíces.
--
-- DEFAULT constante: desde PG 11 el ADD COLUMN es solo metadatos, sin reescribir.
ALTER TABLE messages ADD COLUMN also_in_channel boolean NOT NULL DEFAULT false;

-- El índice del feed (mig. 0001) se limitaba a raíces; ahora el feed también lee las
-- respuestas difundidas. Las consultas que siguen pidiendo `parent_id IS NULL` (rango
-- de raíces, «alrededor de») lo siguen usando: su filtro implica este predicado.
DROP INDEX IF EXISTS messages_channel_feed;
CREATE INDEX messages_channel_feed
    ON messages (tenant, channel_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL AND (parent_id IS NULL OR also_in_channel);
