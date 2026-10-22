-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El PREFIJO de Sytue Web pasa de `/pages` a `/web`: la dirección completa es
-- `https://web.sytue.eu/web/<organización>/…` y, en el host de organización,
-- `https://<slug>.sytue.eu/web/…`. Es la segunda mitad del cambio de dirección
-- (la primera, el host, es la 0104). La clave técnica sigue siendo `pages`.
--
-- El chart de frontends anuncia desde este mismo cambio
-- `https://web.sytue.eu/web/auth/callback` en SYTUE_OIDC_REDIRECT_URI, y OIDC
-- compara la cadena ENTERA: sin esta fila el despliegue sale verde y entrar en
-- Sytue Web falla con `invalid_request`, sin rastro en los pods. La del host de
-- organización la exige el mismo guardián que a las demás
-- (TestRedirectUrisDeLosHostsDeSuiteCubrenElChart, platform/pkg/migcheck).
--
-- SE AÑADEN, NO SE SUSTITUYEN. Los callbacks con `/pages` (0102 y 0104) se
-- quedan: el prefijo anterior redirige al nuevo, y volver a la release anterior
-- —que los anuncia— no tiene que pedir una migración.
--
-- La columna del identificador es `id`, NO `client_id` (ver 0001 y la nota de
-- 0094: un SQL malo aquí no falla en el despliegue, falla al levantar el IdP).
BEGIN;

-- Idempotente como 0033 y 0104: añade únicamente lo que falta.
UPDATE oauth_clients
SET redirect_uris = redirect_uris || ARRAY(
        SELECT u
        FROM unnest(ARRAY[
            -- La CANÓNICA: host de producto con prefijo (la forma de 0099).
            'https://web.sytue.eu/web/auth/callback',
            -- Host de organización (0054): sigue valiendo durante la transición.
            'https://corporate.sytue.eu/web/auth/callback'
        ]) AS u
        WHERE NOT (u = ANY (redirect_uris))
    ),
    post_logout_redirect_uris = post_logout_redirect_uris || ARRAY(
        SELECT u
        FROM unnest(ARRAY[
            'https://web.sytue.eu/web/',
            'https://corporate.sytue.eu/web/'
        ]) AS u
        WHERE NOT (u = ANY (post_logout_redirect_uris))
    ),
    updated_at = now()
WHERE id = 'sytue-pages';

COMMIT;
