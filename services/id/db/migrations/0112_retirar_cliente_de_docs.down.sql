-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelta atrás de 0112: el client de la app conjunta, con la forma que tenía (0002 y
-- las callbacks de 0054 y 0099). Nace sin secreto, como siempre: lo pone el despliegue.
BEGIN;
INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-docs', 'Sytue Docs', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
       'https://docs.sytue.eu/docs/auth/callback',
       'https://docs.sytue.eu/auth/callback',
       'https://corporate.sytue.eu/docs/auth/callback',
       'https://docs.sytue.localhost/auth/callback',
       'http://localhost:4329/auth/callback'
     ],
     ARRAY['https://docs.sytue.eu/', 'http://localhost:4329/'])
ON CONFLICT (id) DO NOTHING;
COMMIT;
