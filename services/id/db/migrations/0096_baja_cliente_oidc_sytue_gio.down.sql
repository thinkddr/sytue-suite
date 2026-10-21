-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelve a dar de alta `sytue-gio` tal como lo sembró la 0070. El secreto NO
-- vuelve (nunca secretos en git): `secret_hash` queda NULL y el token endpoint
-- rechaza al client hasta que el despliegue le fije uno, igual que tras la 0070.
-- Y para que sirva de algo hay que devolver también su audiencia y sus orígenes
-- a platform/pkg/config: esta migración sola no los trae.
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-gio', 'Sytue One (Gio)', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://sytue.com/auth/callback',
        'https://gio.sytue.eu/auth/callback',
        'https://gio.sytue.localhost/auth/callback',
        'http://localhost:8099/auth/callback'
     ],
     ARRAY['https://sytue.com/', 'https://gio.sytue.eu/', 'https://gio.sytue.localhost/', 'http://localhost:8099/'])
ON CONFLICT (id) DO NOTHING;

COMMIT;
