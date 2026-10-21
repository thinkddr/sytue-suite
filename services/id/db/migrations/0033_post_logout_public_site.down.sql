-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

UPDATE oauth_clients
SET post_logout_redirect_uris = array_remove(
        array_remove(post_logout_redirect_uris, 'https://www.sytue.eu/'),
        'https://www.sytue.localhost/'),
    updated_at = now();
