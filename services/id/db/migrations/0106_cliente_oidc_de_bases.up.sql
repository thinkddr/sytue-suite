-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Client confidencial del BFF de Sytue Bases (tablas relacionales, ADR 0068). El secreto
-- se inyecta al desplegar (bootstrap), como en los demás BFF: aquí nace sin él.
--
-- Callbacks: la CANÓNICA es host de producto con prefijo (la forma de 0099,
-- `bases.sytue.eu/bases/auth/callback`, que es la que anuncia el chart de frontends y
-- cruza TestRedirectUrisDeLosHostsDeSuiteCubrenElChart); la raíz del host, por donde
-- entra el BFF; la de organización, mientras ese host siga sirviendo; y las de desarrollo.
--
-- La columna del identificador es `id`, NO `client_id` (ver 0001 y la nota de 0094: un
-- SQL malo aquí no falla en el despliegue, falla al levantar el IdP). Idempotente.
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-bases', 'Sytue Bases', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
       'https://bases.sytue.eu/bases/auth/callback',
       'https://bases.sytue.eu/auth/callback',
       'https://corporate.sytue.eu/bases/auth/callback',
       'https://bases.sytue.localhost/auth/callback',
       'http://localhost:4337/auth/callback'
     ],
     ARRAY[
       'https://bases.sytue.eu/',
       'https://bases.sytue.eu/bases/',
       'https://corporate.sytue.eu/bases/',
       'http://localhost:4337/'
     ])
ON CONFLICT (id) DO NOTHING;

COMMIT;
