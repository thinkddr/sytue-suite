-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Cliente OIDC de la forja privada (git.sytue.eu). Sigue el patrón de
-- `0002_seed_clients`: el secreto NO se siembra aquí, se fija en el despliegue
-- vía Secret Manager, y el token endpoint rechaza autenticación de client
-- mientras secret_hash sea NULL.
--
-- El callback lo impone la forja y no es configurable: /user/oauth2/<nombre del
-- proveedor>/callback, donde el proveedor se registra como `sytue-id`.
--
-- El client pide `openid profile email` porque la forja crea la cuenta local a
-- partir del perfil. La decisión de si ese usuario puede crear organizaciones y
-- repositorios NO se toma aquí: viaja en los claims `roles` y `groups` del ID
-- token, que la forja traduce a usuario normal o restringido.

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-git', 'Sytue Git (forja privada)', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://git.sytue.eu/user/oauth2/sytue-id/callback',
        'https://git.sytue.localhost/user/oauth2/sytue-id/callback'
     ],
     ARRAY['https://git.sytue.eu/'])
ON CONFLICT (id) DO NOTHING;
