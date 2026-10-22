-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte el host canónico de Rooms al estado de la 0002 (Chat y Meet separados).

UPDATE oauth_clients
SET name = 'Sytue Chat',
    redirect_uris = ARRAY[
        'https://chat.sytue.eu/auth/callback',
        'https://chat.sytue.localhost/auth/callback',
        'http://localhost:4330/auth/callback'
    ],
    post_logout_redirect_uris = ARRAY['https://chat.sytue.eu/', 'http://localhost:4330/'],
    updated_at = now()
WHERE id = 'sytue-chat';

UPDATE oauth_clients
SET name = 'Sytue Meet',
    updated_at = now()
WHERE id = 'sytue-meet';
