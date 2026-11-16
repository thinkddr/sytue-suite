-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- POSPONER Y SILENCIAR UNA CONVERSACIÓN, en el servidor.
--
-- Posponer saca el hilo de Recibidos hasta una fecha y lo devuelve entonces, sin
-- leer, como el «Posponer» de Gmail. Silenciar hace que las respuestas nuevas de
-- un hilo que ya no interesa (la cadena de «responder a todos») no vuelvan a la
-- Entrada: van directas al Archivo y no suenan.
--
-- POSPONER es una tabla y no una columna de `threads`: el trabajador que despierta
-- los vencidos los busca en TODAS las organizaciones (app.maintenance, como la cola
-- de salida), y `threads` no tiene ni debe tener política de mantenimiento. Con una
-- tabla aparte, lo único legible entre organizaciones es «qué hilo despierta y
-- cuándo», nunca un asunto ni un mensaje. Mientras dura, los mensajes viven en el
-- buzón de rol `snoozed` (se crea al posponer por primera vez): así un cliente IMAP
-- los ve donde están y la Entrada no los cuenta.
--
-- SILENCIAR sí es una columna de `threads`: es un estado del hilo con su vida, que
-- solo se lee al entregar un correo de esa cuenta, ya con la organización fijada.
-- `threads` ya va con ENABLE y FORCE ROW LEVEL SECURITY desde 0001.

ALTER TABLE threads ADD COLUMN muted boolean NOT NULL DEFAULT false;

CREATE TABLE mail_snoozes (
    thread_id   uuid        PRIMARY KEY REFERENCES threads (id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    account_id  uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    until       timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- El barrido de vencidos va por fecha; la lista de pospuestos de una cuenta, por cuenta.
CREATE INDEX mail_snoozes_until_idx   ON mail_snoozes (until);
CREATE INDEX mail_snoozes_account_idx ON mail_snoozes (tenant, account_id, until);

ALTER TABLE mail_snoozes ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_snoozes FORCE  ROW LEVEL SECURITY;

CREATE POLICY mail_snoozes_tenant_isolation ON mail_snoozes
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- El trabajador que despierta los vencidos reclama filas de cualquier organización
-- (FOR UPDATE SKIP LOCKED, como la liberación de envíos programados) y fija la
-- organización de cada una antes de tocar sus mensajes.
CREATE POLICY mail_snoozes_maintenance ON mail_snoozes
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

-- La de fusión de organizaciones (ver 0010): sin ella, fusionar dos organizaciones
-- dejaría los pospuestos atrás y sus hilos no volverían nunca a la Entrada.
CREATE POLICY mail_snoozes_platform_merge ON mail_snoozes
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
