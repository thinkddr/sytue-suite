-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El client OIDC del BFF de SyERP (`sytue-syerp`).
--
-- Sin esta fila, la superficie de SyERP no puede iniciar sesión: su BFF hace
-- Authorization Code + PKCE contra Sytue ID y el token endpoint rechaza un
-- client_id que no existe. Mismo alta que 0058 dio a Sytue Photos y 0060 a «Mi
-- cuenta»; aquí sola porque el producto es nuevo. Lo pide el chart
-- (`charts/sytue-frontend/values.yaml`, `erp.clientId`), y es exactamente el
-- desajuste que caza `helm-render-test.sh`: el chart nombra un client y ninguna
-- migración lo siembra.
--
-- EL SECRETO NO SE SIEMBRA (nunca secretos en git): `secret_hash` queda NULL y
-- lo fija el despliegue vía Secret Manager, igual que el resto.
--
-- Los cuatro redirect_uri tienen EXACTAMENTE la forma de las demás superficies
-- (ver 0057, 0058 y 0060): raíz del subdominio, la dirección CANÓNICA por
-- organización —que es la que anuncia el chart—, dev con Traefik y dev con Astro
-- directo. Ni una quinta con `/erp/auth/callback` bajo el subdominio propio: el
-- barrido de cierre de sesión deriva una URL por callback, y esa sería la MISMA
-- máquina barrida dos veces. OIDC compara el literal: la que falte es un login
-- que falla con `invalid_request`.
--
-- EL WEBHOOK DE STRIPE NO ENTRA AQUÍ, y conviene dejarlo dicho porque es la
-- pregunta que surge al ver la lista: `erp.sytue.eu/webhooks/stripe/<org>` lo
-- atiende el servicio Go directamente, sin sesión y sin pasar por este client.
-- Su autenticación es la firma HMAC del cuerpo, no un token de Sytue ID.
--
-- La columna del identificador es `id`, NO `client_id` (ver 0001 y la nota de
-- 0057: un SQL malo aquí no falla en el despliegue, falla al levantar el IdP).
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-syerp', 'SyERP', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://erp.sytue.eu/auth/callback',
        'https://corporate.sytue.eu/erp/auth/callback',
        'https://erp.sytue.localhost/auth/callback',
        'http://localhost:4334/auth/callback'
     ],
     ARRAY['https://erp.sytue.eu/', 'http://localhost:4334/'])
ON CONFLICT (id) DO NOTHING;

COMMIT;
