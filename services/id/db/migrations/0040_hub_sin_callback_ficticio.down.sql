-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Restaura el callback ficticio y el grant del seed 0002 (los dos entornos, como
-- allí). No los usa nadie: esto solo existe para que la migración sea reversible.
UPDATE oauth_clients
SET redirect_uris = ARRAY[
        'https://hub.sytue.eu/auth/callback',
        'https://hub.sytue.localhost/auth/callback'
    ],
    grant_types = ARRAY['authorization_code', 'refresh_token', 'client_credentials'],
    updated_at = now()
WHERE id = 'sytue-hub';
