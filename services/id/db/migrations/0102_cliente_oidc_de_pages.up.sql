-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- Client confidencial del BFF de Sytue Pages. El secreto se inyecta al desplegar.
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-pages', 'Sytue Pages', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
       'https://pages.sytue.eu/auth/callback',
       'https://corporate.sytue.eu/pages/auth/callback',
       'https://pages.sytue.eu/pages/auth/callback',
       'https://pages.sytue.localhost/auth/callback',
       'http://localhost:4336/auth/callback'
     ],
     ARRAY[
       'https://pages.sytue.eu/',
       'https://pages.sytue.eu/pages/',
       'https://corporate.sytue.eu/pages/',
       'http://localhost:4336/'
     ])
ON CONFLICT (id) DO NOTHING;

COMMIT;
