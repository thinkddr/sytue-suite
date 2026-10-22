-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El client OIDC de las apps de ESCRITORIO nativas (`sytue-desktop`), el shell
-- Go/Fyne compartido por los bundles de Windows/Linux/macOS-Go.
--
-- Es el PRIMER cliente PUBLICO del producto (auth method "none", PKCE puro):
-- un binario instalado en el portátil del usuario no puede guardar un secreto,
-- y el flujo es el del nivel B de OAuth para apps nativas — Authorization Code
-- + PKCE con redirect al LOOPBACK fijo http://127.0.0.1:8921/auth/callback,
-- que levanta el propio shell al conectar. Mismo alta que 0057/0058 dieron a
-- las superficies web, pública y sin secreto que sembrar en el despliegue.
--
-- La columna del identificador es `id`, NO `client_id` (ver 0057).
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-desktop', 'Sytue Escritorio', 'public', true, 'none',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY['http://127.0.0.1:8921/auth/callback'],
     ARRAY[]::text[])
ON CONFLICT (id) DO NOTHING;

COMMIT;
