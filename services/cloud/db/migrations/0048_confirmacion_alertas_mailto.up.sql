-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

-- Confirmación por enlace de las direcciones mailto: de las alertas de Cloud
-- (auditoría 2026-10-06, CLOUD-19).
--
-- Un webhook HTTP(S) nace confirmado (`confirmed_at` relleno al crear); un
-- canal `mailto:` nace con `confirmed_at IS NULL` y `confirm_token_hash` con el
-- SHA-256 hex del token enviado al destinatario, y no recibe avisos hasta que
-- quien recibe ese correo abre el enlace de confirmación.

ALTER TABLE alert_policies
    ADD COLUMN confirmed_at       timestamptz,
    ADD COLUMN confirm_token_hash text;

CREATE UNIQUE INDEX alert_policies_confirm_token_hash_idx
    ON alert_policies (confirm_token_hash)
    WHERE confirm_token_hash IS NOT NULL;

-- La confirmación por enlace llega sin sesión OIDC: el token de 256 bits es la
-- credencial y resuelve la fila bajo pg.InMaintenanceTx (`app.maintenance='on'`).
CREATE POLICY alert_policies_maintenance ON alert_policies
    USING (current_setting('app.maintenance', true) = 'on')
    WITH CHECK (current_setting('app.maintenance', true) = 'on');

SELECT set_config('app.maintenance', 'on', false);

UPDATE alert_policies
   SET confirmed_at = created_at
 WHERE channel_url NOT LIKE 'mailto:%';

SELECT set_config('app.maintenance', '', false);
