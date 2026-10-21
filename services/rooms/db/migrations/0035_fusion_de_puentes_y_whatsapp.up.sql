-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Política de FUSIÓN para las tablas de puentes (0033) y WhatsApp (0034).
--
-- Llegaron después de la 0021, que generó la política para las tablas que había
-- entonces, y ninguna migración se la dio: la fusión de organizaciones aborta en su
-- preflight y `platform/pkg/migcheck` está en rojo desde que se fusionaron. Van
-- escritas una a una, como pide el gate, que lee el SQL sin ejecutarlo.

CREATE POLICY channel_bridges_platform_merge ON channel_bridges
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY whatsapp_accounts_platform_merge ON whatsapp_accounts
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY whatsapp_chats_platform_merge ON whatsapp_chats
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE POLICY whatsapp_seen_platform_merge ON whatsapp_seen
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
