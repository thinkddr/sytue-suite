-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- WHATSAPP BUSINESS (Cloud API de Meta): un número de la organización conectado a
-- un equipo de Rooms. Cada cliente que escribe tiene su canal en ese equipo y lo que
-- el equipo contesta allí le llega por WhatsApp.
--
-- SEGURIDAD:
--   * token_hash: la URL del webhook lleva un token opaco (como /hooks/{token}) que
--     también es el «token de verificación» que pide Meta. Se guarda su SHA-256.
--   * access_token y app_secret van cifrados con keycrypto (`encv1:`). Además, cada
--     aviso se comprueba con X-Hub-Signature-256 (HMAC del cuerpo con app_secret).
--   * El aviso llega sin sesión ni tenant: la búsqueda por token es CROSS-TENANT por
--     la política de mantenimiento, igual que incoming_webhooks (mig. 0014).
CREATE TABLE whatsapp_accounts (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    team_id         uuid        NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    phone_number_id text        NOT NULL,
    display_phone   text        NOT NULL DEFAULT '',
    verified_name   text        NOT NULL DEFAULT '',
    access_token    text        NOT NULL,
    app_secret      text        NOT NULL,
    token_hash      text        NOT NULL,
    created_by      text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX whatsapp_accounts_token_hash ON whatsapp_accounts (token_hash);
-- Un número no se conecta dos veces en la misma organización.
CREATE UNIQUE INDEX whatsapp_accounts_phone ON whatsapp_accounts (tenant, phone_number_id);

-- Un cliente de una cuenta = un canal.
CREATE TABLE whatsapp_chats (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant          text        NOT NULL,
    account_id      uuid        NOT NULL REFERENCES whatsapp_accounts (id) ON DELETE CASCADE,
    wa_id           text        NOT NULL,
    channel_id      uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    contact_name    text        NOT NULL DEFAULT '',
    last_inbound_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, wa_id)
);
-- El consumidor del outbox pregunta por canal en cada mensaje.
CREATE INDEX whatsapp_chats_by_channel ON whatsapp_chats (tenant, channel_id);

-- Meta entrega al menos una vez: el id de mensaje (wamid) visto evita publicarlo dos
-- veces. Se purga a los 7 días al insertar (índice por fecha).
CREATE TABLE whatsapp_seen (
    tenant     text        NOT NULL,
    message_id text        NOT NULL,
    seen_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, message_id)
);
CREATE INDEX whatsapp_seen_by_date ON whatsapp_seen (tenant, seen_at);

ALTER TABLE whatsapp_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE whatsapp_accounts FORCE ROW LEVEL SECURITY;
ALTER TABLE whatsapp_chats ENABLE ROW LEVEL SECURITY;
ALTER TABLE whatsapp_chats FORCE ROW LEVEL SECURITY;
ALTER TABLE whatsapp_seen ENABLE ROW LEVEL SECURITY;
ALTER TABLE whatsapp_seen FORCE ROW LEVEL SECURITY;

CREATE POLICY whatsapp_accounts_tenant_isolation ON whatsapp_accounts
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY whatsapp_accounts_maintenance ON whatsapp_accounts
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');
CREATE POLICY whatsapp_chats_tenant_isolation ON whatsapp_chats
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
CREATE POLICY whatsapp_seen_tenant_isolation ON whatsapp_seen
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
