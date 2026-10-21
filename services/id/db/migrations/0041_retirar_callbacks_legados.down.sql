-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Devuelve los callbacks legados a los dos clients que los tenían (0023 y 0026),
-- para volver al estado anterior si hubiera que revertir el renombrado.
-- Idempotente: añade solo lo que falte.
UPDATE oauth_clients
SET redirect_uris = redirect_uris || ARRAY(
        SELECT u FROM unnest(ARRAY[
            'https://chat.sytue.eu/auth/callback',
            'https://chat.sytue.localhost/auth/callback',
            'https://meet.sytue.eu/auth/callback',
            'https://meet.sytue.localhost/auth/callback'
        ]) AS u
        WHERE NOT (u = ANY (redirect_uris))
    ),
    updated_at = now()
WHERE id = 'sytue-chat';

UPDATE oauth_clients
SET redirect_uris = redirect_uris || ARRAY(
        SELECT u FROM unnest(ARRAY[
            'https://console.sytue.eu/auth/callback',
            'https://console.sytue.localhost/auth/callback'
        ]) AS u
        WHERE NOT (u = ANY (redirect_uris))
    ),
    updated_at = now()
WHERE id = 'sytue-console';
