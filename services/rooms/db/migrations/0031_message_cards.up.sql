-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- OBJETOS RICOS en el mensaje: encuesta y tarea (Rooms · «objetos ricos en el chat»).
--
-- Un objeto rico NO es un tipo de mensaje nuevo: es un mensaje normal con una
-- TARJETA adjunta. El texto sigue en `body` —la pregunta de la encuesta, el título
-- de la tarea—, así que la búsqueda, las notificaciones, el push y cualquier cliente
-- que no entienda tarjetas siguen enseñando algo legible sin tocar una línea. La
-- columna `card` lleva solo lo que el texto no puede: las opciones, si admite varias
-- respuestas, a quién se asigna la tarea y para cuándo.
--
-- Por eso tampoco se toca `messages_type_valid` ni `messages_body_not_blank`: una
-- encuesta o una tarea son type='user' con cuerpo no vacío, que es lo que esos
-- invariantes ya exigen.
--
-- Backfill: ADD COLUMN ... NOT NULL DEFAULT constante lo resuelve PostgreSQL por
-- metadatos, sin reescribir la tabla (barato a escala). messages ya tiene RLS por
-- tenant (0001_init) y la columna nueva la hereda.

ALTER TABLE messages ADD COLUMN card jsonb NOT NULL DEFAULT '{}'::jsonb;

-- ---------------------------------------------------------------------------
-- Estado de la tarjeta: una fila por (mensaje, usuario, valor). El voto de una
-- encuesta es el índice de la opción ('0'..'9'); la tarea hecha es el valor
-- 'done'. UNA tabla para los dos porque el estado es el mismo hecho —«esta
-- persona marcó este valor»— y quien lo marcó importa en ambos casos: en la
-- encuesta para enseñar quién votó qué, en la tarea para saber quién la cerró.
--
-- Se agrega en la lectura de mensajes con una sola consulta ANY($ids), igual que
-- reactions y message_acks: sin N+1. La PK (tenant, message_id, subject, value)
-- es a la vez el índice que cubre esa agregación (prefijo tenant, message_id).
-- ---------------------------------------------------------------------------
CREATE TABLE message_card_votes (
    tenant     text        NOT NULL,
    message_id uuid        NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    subject    text        NOT NULL,
    value      text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, message_id, subject, value),
    CONSTRAINT message_card_votes_value_len CHECK (length(value) BETWEEN 1 AND 16)
);

-- RLS deny-by-default por tenant (mismo patrón que reactions/message_acks).
ALTER TABLE message_card_votes ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_card_votes FORCE ROW LEVEL SECURITY;
CREATE POLICY message_card_votes_tenant_isolation ON message_card_votes
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Política de FUSIÓN de organizaciones: toda tabla con columna `tenant` la necesita.
-- Sin ella, mover una organización aborta en el preflight —o peor, se da por completada
-- dejando atrás los votos, que reaparecerían a cero en la organización destino—. Va
-- dentro de esta misma migración porque es nueva y no está aplicada en ningún entorno
-- (a una migración ya aplicada habría que añadírsela en un fichero aparte, para no
-- cambiar su suma de comprobación).
CREATE POLICY message_card_votes_platform_merge ON message_card_votes
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
