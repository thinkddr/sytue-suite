-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El client OIDC del BFF de «Mi cuenta» (`sytue-account`).
--
-- La pantalla era HTML servido por el propio Sytue ID y ahora es una app Astro
-- con su BFF, así que necesita client propio: sin esta fila el token endpoint
-- rechaza el `client_id` y nadie entra. Misma alta que 0058 dio a Sytue Photos.
--
-- EL SECRETO NO SE SIEMBRA (nunca secretos en git): `secret_hash` queda NULL y
-- lo fija el despliegue vía Secret Manager, igual que el resto.
--
-- Cuatro redirect_uri con la forma de las demás superficies: raíz del
-- subdominio, la dirección CANÓNICA por organización —que es la que anuncia el
-- chart—, dev con Traefik y dev con Astro directo. Ni una quinta bajo
-- `account.sytue.eu/account`: el barrido de cierre de sesión deriva una URL por
-- callback y esa sería la MISMA máquina barrida dos veces.
--
-- La columna del identificador es `id`, NO `client_id` (ver 0001 y la nota de
-- 0057: un SQL malo aquí no falla en el despliegue, falla al levantar el IdP).
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-account', 'Mi cuenta', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://account.sytue.eu/auth/callback',
        'https://corporate.sytue.eu/account/auth/callback',
        'https://account.sytue.localhost/auth/callback',
        'http://localhost:4333/auth/callback'
     ],
     ARRAY['https://account.sytue.eu/', 'http://localhost:4333/'])
ON CONFLICT (id) DO NOTHING;

COMMIT;
