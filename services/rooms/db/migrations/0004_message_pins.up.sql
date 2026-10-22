-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Fijados (pinned): mensajes destacados por canal. Cualquier miembro del canal
-- puede fijar/desfijar (convención Mattermost por defecto; permiso = acceso al
-- canal, igual que reaccionar). Un mensaje se fija una sola vez (PK por mensaje);
-- vive en un único canal, así que channel_id se denormaliza para indexar el
-- listado por canal sin volver a la tabla messages.

CREATE TABLE message_pins (
    tenant     text        NOT NULL,
    message_id uuid        NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    channel_id uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    pinned_by  text        NOT NULL,
    pinned_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, message_id)
);

-- Listado de fijados de un canal en orden (más reciente primero) con índice
-- que cubre exactamente el WHERE + ORDER BY de ListPins (sin seq scan).
CREATE INDEX message_pins_by_channel
    ON message_pins (tenant, channel_id, pinned_at DESC, message_id DESC);

-- RLS deny-by-default por tenant (mismo patrón que el resto del esquema).
ALTER TABLE message_pins ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_pins FORCE ROW LEVEL SECURITY;
CREATE POLICY message_pins_tenant_isolation ON message_pins
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
