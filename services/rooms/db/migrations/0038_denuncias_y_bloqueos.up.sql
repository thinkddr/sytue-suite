-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- DENUNCIAR y BLOQUEAR en Rooms (ADR 0073). Hasta aquí no existía ni lo uno ni
-- lo otro, y una aplicación cuyo contenido es de sus usuarios tiene que dar las
-- dos cosas: es lo que Google Play pregunta y exige.
--
-- UNA DENUNCIA es de la organización que ALOJA la conversación (ADR 0040): quien
-- viene de fuera denuncia ahí, y actúan sus administradores. La fila ES la traza:
-- quién, a quién, sobre qué y cuándo; y quién la resolvió, cómo y cuándo. Se
-- resuelve UNA vez (el UPDATE exige status='open') y no se borra. Del mensaje se
-- guarda el id, NO el texto: la cola lo lee en vivo y, si ya se borró, no lo ve.
CREATE TABLE message_reports (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant       text        NOT NULL,
    reporter_sub text        NOT NULL,
    target_sub   text        NOT NULL CHECK (target_sub <> reporter_sub),
    target_name  text        NOT NULL DEFAULT '',
    channel_id   uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    -- Sin clave ajena a propósito: la denuncia sobrevive al mensaje.
    message_id   uuid,
    reason       text        NOT NULL CHECK (reason IN ('harassment', 'sexual', 'violence', 'spam', 'other')),
    note         text        NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    status       text        NOT NULL DEFAULT 'open'
                 CHECK (status IN ('open', 'dismissed', 'message_deleted', 'member_removed')),
    resolved_by  text,
    resolved_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- Deduplicación: una persona denuncia UNA vez cada mensaje, y a otra persona en
-- un canal una vez mientras esa denuncia siga abierta. Repetirla devuelve la que
-- ya había, sin avisar otra vez a nadie: denunciar no sirve para acosar.
CREATE UNIQUE INDEX message_reports_un_mensaje ON message_reports (tenant, reporter_sub, message_id)
    WHERE message_id IS NOT NULL;
CREATE UNIQUE INDEX message_reports_una_persona ON message_reports (tenant, reporter_sub, channel_id, target_sub)
    WHERE message_id IS NULL AND status = 'open';
CREATE INDEX message_reports_cola ON message_reports (tenant, status, created_at DESC);
-- El tope diario por persona se cuenta con este.
CREATE INDEX message_reports_por_denunciante ON message_reports (tenant, reporter_sub, created_at);

ALTER TABLE message_reports ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_reports FORCE ROW LEVEL SECURITY;

CREATE POLICY message_reports_tenant_isolation ON message_reports
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

CREATE POLICY message_reports_platform_merge ON message_reports
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- UN BLOQUEO es de la PERSONA, no de la organización: quien bloquea a alguien no
-- quiere sus mensajes directos ni sus avisos tampoco en la conversación que
-- comparten en otra organización. Por eso la clave es el par de personas —el
-- `sub` de Sytue ID es el mismo en todas— y `tenant` solo dice dónde se hizo.
--
-- Bloquear NO borra nada: los mensajes siguen en la conversación, con la
-- retención de la organización que la aloja.
CREATE TABLE user_blocks (
    tenant       text        NOT NULL,
    -- Quien bloquea. Se llama `subject` como en el resto de tablas personales: es
    -- por donde el borrado de cuenta (services/id/internal/merge) las encuentra.
    subject      text        NOT NULL,
    blocked_sub  text        NOT NULL CHECK (blocked_sub <> subject),
    -- El nombre con el que se le veía al bloquearle, para la lista de ajustes.
    blocked_name text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subject, blocked_sub)
);

-- «¿Quién de estos ha bloqueado al autor?», al entregar un mensaje.
CREATE INDEX user_blocks_por_bloqueado ON user_blocks (blocked_sub);

ALTER TABLE user_blocks ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_blocks FORCE ROW LEVEL SECURITY;

CREATE POLICY user_blocks_tenant_isolation ON user_blocks
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- El bloqueo se aplica y se gestiona ENTRE organizaciones, así que el servidor lo
-- lee y lo escribe por la política de mantenimiento, siempre por el `sub` del
-- token verificado (store/moderacion.go). Una transacción de negocio no la fija.
CREATE POLICY user_blocks_maintenance ON user_blocks
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

CREATE POLICY user_blocks_platform_merge ON user_blocks
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
