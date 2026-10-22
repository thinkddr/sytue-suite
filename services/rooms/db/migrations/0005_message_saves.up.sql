-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Guardados (saved): marcadores por-USUARIO de mensajes, cross-canal. La vista
-- «Guardados» lista, por fecha de guardado (keyset descendente), los mensajes que
-- el propio usuario guardó. channel_id se denormaliza para reaplicar el filtro de
-- acceso al canal al listar (no devolver un mensaje de un canal privado del que ya
-- no eres miembro), como hace SearchMessages.

CREATE TABLE message_saves (
    tenant     text        NOT NULL,
    subject    text        NOT NULL,
    message_id uuid        NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    channel_id uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, subject, message_id)
);

-- Keyset de la vista «Guardados» por usuario: (tenant, subject, created_at DESC).
-- Cubre el WHERE (tenant+subject) y el ORDER BY (created_at DESC, message_id DESC)
-- de ListSaved; coste O(página), no O(guardados) — escala a cientos de miles.
CREATE INDEX message_saves_by_subject
    ON message_saves (tenant, subject, created_at DESC, message_id DESC);

-- RLS deny-by-default por tenant (mismo patrón que el resto del esquema).
ALTER TABLE message_saves ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_saves FORCE ROW LEVEL SECURITY;
CREATE POLICY message_saves_tenant_isolation ON message_saves
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
