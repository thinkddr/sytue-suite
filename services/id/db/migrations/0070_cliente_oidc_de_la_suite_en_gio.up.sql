-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El client OIDC de la suite en Gio para la web (`sytue-gio`): el BFF de
-- services/gioweb, que sirve la versión WebAssembly en `sytue.com` mientras
-- `sytue.eu` sigue sirviendo la de Astro + Preact (ADR 0045).
--
-- Es CONFIDENCIAL, como los BFF de las superficies web (0057) y al revés que
-- `sytue-desktop` (0069): aquí hay un servidor que sí puede guardar un secreto,
-- y el WebAssembly no ve ni el secreto ni los tokens.
--
-- EL SECRETO NO SE SIEMBRA (nunca secretos en git): `secret_hash` queda NULL y
-- lo fija el despliegue, igual que el resto. Mientras tanto el token endpoint
-- rechaza a este client, que es la degradación correcta.
--
-- UN solo client para toda la suite, y no uno por app: la versión en Gio es UNA
-- página que cambia de app sin recargar, con UNA sesión. Su audiencia entra en
-- platform/pkg/config.firstPartyAudiences en este mismo commit; sin eso, todos
-- los servicios le contestan 401 y se ve como pantallas vacías (lo que le pasó
-- a Sytue Fotos el 2026-09-10).
--
-- Los redirect_uri: `sytue.com`, que es donde queda al completarse el puerto;
-- `gio.sytue.eu`, donde vive mientras tanto —ya entra por el comodín del frontal—;
-- y dev. El frente tiene UN host canónico (su cookie de sesión es `__Host-`), así
-- que aquí no hay dirección por organización: gioweb redirige los alias a él.
-- Una instalación con otro dominio añade la suya en bootstrap-id.sh (GIO_HOST).
--
-- La columna del identificador es `id`, NO `client_id` (ver 0057).
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
