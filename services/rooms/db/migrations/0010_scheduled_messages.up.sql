-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Mensajes PROGRAMADOS (Ola 12 · Profundidad de mensaje). El usuario redacta un
-- mensaje y fija un `send_at` futuro; un WORKER (cmd/chat) escanea los vencidos
-- (`send_at <= now()`), los publica reusando el flujo de PostMessage y borra la
-- fila. La cola solo guarda los PENDIENTES (se borran al enviar), así que crece
-- acotada por el propio drenaje — como calendar_reminders (mig. calendar 0002).
--
-- parent_id permite programar una respuesta en hilo (NULL = mensaje de canal).
-- file_ids se resuelve a adjuntos en el envío (mismo flujo que PostMessage).

CREATE TABLE scheduled_messages (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    channel_id  uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    -- Respuesta programada en hilo (raíz). NULL = mensaje de canal. Si la raíz se
    -- borra en duro, la fila cae con ella (la validación real es al enviar).
    parent_id   uuid        REFERENCES messages (id) ON DELETE CASCADE,
    author_sub  text        NOT NULL,
    author_name text        NOT NULL DEFAULT '',
    body        text        NOT NULL DEFAULT '',
    -- Ficheros de Sytue Drive a adjuntar (se resuelven al enviar, como PostMessage).
    file_ids    uuid[]      NOT NULL DEFAULT '{}',
    -- Instante (UTC) en que el worker debe publicar el mensaje.
    send_at     timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Cola del worker: los vencidos ordenados por disparo. La tabla solo tiene
-- pendientes (se borran al enviar), así que un índice llano sobre (send_at) cubre
-- el WHERE send_at<=now() ORDER BY send_at del drenaje — O(página) por tick,
-- escala a cientos de miles.
CREATE INDEX scheduled_messages_due_idx ON scheduled_messages (send_at);

-- Keyset de «mis programados» por usuario (GET /me/scheduled), ordenados por
-- send_at ascendente. Cubre WHERE (tenant, author_sub) + ORDER BY (send_at, id).
CREATE INDEX scheduled_messages_by_author
    ON scheduled_messages (tenant, author_sub, send_at, id);

-- ---------------------------------------------------------------------------
-- RLS: aislamiento por tenant + política PERMISSIVE de mantenimiento para que el
-- worker (cross-tenant, sin `sub` de usuario) drene la cola con `app.maintenance`
-- fijado (pg.InMaintenanceTx), sin necesidad de un rol BYPASSRLS. El camino de
-- usuario (crear/listar/editar/cancelar, InTenantTx) usa la política por tenant.
-- ---------------------------------------------------------------------------
ALTER TABLE scheduled_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE scheduled_messages FORCE ROW LEVEL SECURITY;

CREATE POLICY scheduled_messages_tenant_isolation ON scheduled_messages
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY scheduled_messages_maintenance ON scheduled_messages
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
