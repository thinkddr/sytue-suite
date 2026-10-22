-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte el host canónico de Admin al estado de la 0002 (producto «Console»).

UPDATE oauth_clients
SET name = 'Sytue Console',
    redirect_uris = ARRAY[
        'https://console.sytue.eu/auth/callback',
        'https://console.sytue.localhost/auth/callback',
        'http://localhost:4322/auth/callback'
    ],
    post_logout_redirect_uris = ARRAY['https://console.sytue.eu/', 'http://localhost:4322/'],
    updated_at = now()
WHERE id = 'sytue-console';
