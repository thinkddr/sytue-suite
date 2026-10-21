-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El client OIDC del BFF de Sytue Notepads (`sytue-notes`).
--
-- Sin esta fila, la superficie de Notes no puede iniciar sesión: su BFF hace
-- Authorization Code + PKCE contra Sytue ID y el token endpoint rechaza un
-- client_id que no existe. Es el mismo alta que tuvieron en su día las otras
-- diez superficies (0002_seed_clients), aquí sola porque el producto es nuevo.
--
-- EL SECRETO NO SE SIEMBRA (nunca secretos en git): `secret_hash` queda NULL y
-- lo fija el despliegue vía Secret Manager, igual que el resto. Mientras tanto
-- el token endpoint rechaza la autenticación de este client, que es la
-- degradación correcta.
--
-- Los tres redirect_uri cubren prod (`notes.sytue.eu`), dev con Traefik
-- (`notes.sytue.localhost`) y dev con Astro directo (`localhost:4331`); el
-- cuarto es la dirección CANÓNICA por organización, que es la que anuncia el
-- chart en `SYTUE_OIDC_REDIRECT_URI` (ver 0054). OIDC compara el literal, así
-- que la que falte es un login que falla con `invalid_request` y sin más pista.
--
-- OJO: la columna del identificador de cliente se llama `id`, NO `client_id`
-- (ver 0001_core_identity.up.sql). Confundirlas dejó una vez la base marcada
-- como dirty y el IdP en CrashLoopBackOff, porque `pg.MigrateUp` corre al
-- arrancar: un SQL malo no falla en el despliegue, falla al levantar.
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-notes', 'Sytue Notepads', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://notes.sytue.eu/auth/callback',
        'https://corporate.sytue.eu/notes/auth/callback',
        'https://notes.sytue.localhost/auth/callback',
        'http://localhost:4331/auth/callback'
     ],
     ARRAY['https://notes.sytue.eu/', 'http://localhost:4331/'])
ON CONFLICT (id) DO NOTHING;

COMMIT;
