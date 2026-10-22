-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- PUENTES de canal: los mensajes de un canal de Rooms se repiten en un canal de
-- Slack, Microsoft Teams o Google Chat por el webhook entrante de cada uno. La
-- vuelta ya existe: el webhook entrante de Rooms entiende los tres formatos.
--
-- webhook_url es un SECRETO (quien la tiene publica en el canal ajeno): se guarda
-- cifrada con keycrypto (`encv1:`), como los secretos de firma (mig. 0025). El
-- servicio valida que el host sea el del proveedor antes de guardarla.
CREATE TABLE channel_bridges (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant        text        NOT NULL,
    channel_id    uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    platform      text        NOT NULL CHECK (platform IN ('slack', 'teams', 'googlechat')),
    webhook_url   text        NOT NULL,
    target        text        NOT NULL DEFAULT '',
    -- Último fallo de entrega, para que el admin vea en la lista que el otro lado
    -- dejó de aceptar (webhook revocado, flujo borrado). Vacío = entregando.
    last_error    text        NOT NULL DEFAULT '',
    last_error_at timestamptz,
    created_by    text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX channel_bridges_by_channel ON channel_bridges (tenant, channel_id);

ALTER TABLE channel_bridges ENABLE ROW LEVEL SECURITY;
ALTER TABLE channel_bridges FORCE ROW LEVEL SECURITY;

CREATE POLICY channel_bridges_tenant_isolation ON channel_bridges
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
