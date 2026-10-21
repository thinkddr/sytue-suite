-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El client OIDC del BFF de Sytue Photos (`sytue-syphotos`).
--
-- Sin esta fila, la superficie de Sytue Photos no puede iniciar sesión: su BFF hace
-- Authorization Code + PKCE contra Sytue ID y el token endpoint rechaza un
-- client_id que no existe. Mismo alta que 0057 dio a Sytue Notepads, aquí sola
-- porque el producto es nuevo. Lo cazó helm-render-test.sh: el chart pedía el
-- client y ninguna migración lo sembraba.
--
-- EL SECRETO NO SE SIEMBRA (nunca secretos en git): `secret_hash` queda NULL y
-- lo fija el despliegue vía Secret Manager, igual que el resto.
--
-- Los cuatro redirect_uri tienen EXACTAMENTE la forma de las otras once
-- superficies (ver 0057): raíz del subdominio, la dirección CANÓNICA por
-- organización —que es la que anuncia el chart—, dev con Traefik y dev con
-- Astro directo. Se registró una quinta con `/photos/auth/callback` en el
-- subdominio y se retiró: el barrido de cierre de sesión deriva una URL por
-- callback, y esa quinta era la MISMA máquina barrida dos veces. OIDC compara
-- el literal: la que falte es un login que falla con `invalid_request`.
--
-- La columna del identificador es `id`, NO `client_id` (ver 0001 y la nota de
-- 0057: un SQL malo aquí no falla en el despliegue, falla al levantar el IdP).
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-syphotos', 'Sytue Photos', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://photos.sytue.eu/auth/callback',
        'https://corporate.sytue.eu/photos/auth/callback',
        'https://photos.sytue.localhost/auth/callback',
        'http://localhost:4332/auth/callback'
     ],
     ARRAY['https://photos.sytue.eu/', 'http://localhost:4332/'])
ON CONFLICT (id) DO NOTHING;

COMMIT;
