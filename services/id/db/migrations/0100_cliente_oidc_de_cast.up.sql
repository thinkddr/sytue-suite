-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El client OIDC del BFF de Sytue Cast (`sytue-cast`), y su esquema nativo en el
-- client de las apps móviles (ADR 0057, CAST-ENCARGO §2 y §13.4).
--
-- Sin esta fila, la superficie de Sytue Cast no puede iniciar sesión: su BFF hace
-- Authorization Code + PKCE contra Sytue ID y el token endpoint rechaza un
-- client_id que no existe. Mismo alta que 0058 dio a Sytue Photos, y con las DOS
-- formas que llegaron después de aquella: la CANÓNICA por host de producto con
-- prefijo (0099, `https://cast.sytue.eu/cast/auth/callback`, que es la que el
-- chart anuncia en SYTUE_OIDC_REDIRECT_URI) y la del frente en `sytue.com` (0094).
-- Van aquí y no en esas migraciones porque golang-migrate solo aplica versiones
-- nuevas: 0094 y 0099 ya están aplicadas en producción y no volverán a correr.
--
-- EL SECRETO NO SE SIEMBRA (nunca secretos en git): `secret_hash` queda NULL y
-- lo fija el despliegue —bin/bootstrap-id.sh en el piloto (OIDC_CAST), Secret
-- Manager en producción— igual que el resto.
--
-- Los seis redirect_uri son literales: OIDC compara la cadena entera, y la que
-- falte es un login que falla con `invalid_request` en ESE host y sin rastro en
-- los pods. Los guardianes de platform/pkg/migcheck
-- (TestRedirectUrisDeLosHostsDeSuiteCubrenElChart y
-- TestRedirectUrisDeSytueComCubrenElCompose) cruzan esta lista con el chart de
-- frontends y con el compose de sytue.com.
--
-- La columna del identificador es `id`, NO `client_id` (ver 0001 y la nota de
-- 0094: un SQL malo aquí no falla en el despliegue, falla al levantar el IdP).
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-cast', 'Sytue Cast', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        -- Raíz del subdominio, como el resto de superficies (0057/0058).
        'https://cast.sytue.eu/auth/callback',
        -- Host de organización (0054): sigue valiendo durante la transición.
        'https://corporate.sytue.eu/cast/auth/callback',
        -- La CANÓNICA: host de producto con prefijo (0099).
        'https://cast.sytue.eu/cast/auth/callback',
        -- El frente en sytue.com (0094; deploy/sytue-com/docker-compose.yml).
        'https://cast.sytue.com/cast/auth/callback',
        -- Dev con Traefik (deploy/compose) y dev con Astro directo (puerto 4335).
        'https://cast.sytue.localhost/auth/callback',
        'http://localhost:4335/auth/callback'
     ],
     -- El cierre de sesión devuelve a la portada pública de Cast (que sí existe
     -- sin sesión: /cast/explore) o al sitio público de cada frente; sin registrarlo,
     -- Sytue ID rechaza el `post_logout_redirect_uri` como open-redirect (0033, 0094).
     ARRAY[
        'https://cast.sytue.eu/',
        'https://cast.sytue.eu/cast/',
        'https://corporate.sytue.eu/cast/',
        'https://www.sytue.com/',
        'http://localhost:4335/'
     ])
ON CONFLICT (id) DO NOTHING;

-- La app nativa de Cast (apps/fyne/cmd/cast, esquema `sytue-cast`) vuelve del
-- navegador del sistema por `sytue-cast://auth/callback`, dentro del client ÚNICO
-- de las apps móviles (0071). `array_append` solo si no está ya: la migración
-- tiene que poder repetirse sin duplicar entradas (pg.MigrateUp corre al arrancar
-- cada réplica).
UPDATE oauth_clients
SET redirect_uris = array_append(redirect_uris, 'sytue-cast://auth/callback')
WHERE id = 'sytue-mobile'
  AND NOT ('sytue-cast://auth/callback' = ANY (redirect_uris));

COMMIT;
