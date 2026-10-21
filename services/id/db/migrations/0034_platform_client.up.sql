-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Client OIDC de platform.sytue.eu, la administración GLOBAL de la plataforma.
--
-- Es un client PROPIO y no el de la Console a propósito: administrar Sytue no
-- debe compartir sesión ni consentimiento con administrar una organización
-- cliente. Comparte binario con la Console (misma app Astro), pero el aislamiento
-- que importa es el de la credencial y la cookie, y ese es por host y por client.
--
-- El `secret_hash` queda NULL como el resto de clients de primera parte: lo fija
-- el despliegue (`id reconcile-client-secrets`), nunca una migración.
INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes, redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-platform', 'Sytue Platform (administración global)', 'confidential', true,
     'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}',
     '{openid,profile,email,offline_access,admin}',
     ARRAY[
        'https://platform.sytue.eu/auth/callback',
        'https://platform.sytue.localhost/auth/callback',
        'http://localhost:4322/auth/callback'
     ],
     ARRAY[
        'https://platform.sytue.eu/',
        'https://platform.sytue.localhost/',
        'http://localhost:4322/'
     ])
ON CONFLICT (id) DO NOTHING;
