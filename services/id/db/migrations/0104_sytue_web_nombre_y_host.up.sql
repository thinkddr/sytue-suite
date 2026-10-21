-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Pages pasa a llamarse SYTUE WEB y su dirección pasa de `pages.sytue.eu` a
-- `web.sytue.eu` (decisión del dueño, 2026-10-04). Cambian el NOMBRE y la
-- DIRECCIÓN; la clave técnica no: el client sigue siendo `sytue-pages`, que es
-- clave ajena de sesiones, tokens y consentimientos (mismo criterio que 0023,
-- que renombró `sytue-chat` a «Sytue Rooms» conservando el id).
--
-- Va en una migración NUEVA y no en la 0102: golang-migrate solo aplica versiones
-- nuevas, y la 0102 ya está aplicada en producción — editarla no cambiaría nada
-- allí y dejaría dos historias distintas según cuándo se levantó cada base.
--
-- QUÉ HACE
--   1. El nombre: es el que enseña la pantalla de consentimiento y la consola.
--   2. Los callbacks del host nuevo. El chart de frontends anuncia desde este
--      mismo cambio `https://web.sytue.eu/pages/auth/callback` en
--      SYTUE_OIDC_REDIRECT_URI, y OIDC compara la cadena ENTERA: sin esta fila el
--      despliegue sale verde y entrar en Sytue Web falla con `invalid_request`,
--      sin rastro en los pods. Lo cruza con el chart
--      TestRedirectUrisDeLosHostsDeSuiteCubrenElChart (platform/pkg/migcheck).
--
-- SE AÑADEN, NO SE SUSTITUYEN. Los de `pages.sytue.eu` (0102) se quedan: ese host
-- sigue sirviendo la app hasta que el despliegue lo convierta en redirección
-- (`redirigirHostAnterior`, apagado por defecto), y un login empezado allí tiene
-- que poder terminar. Tampoco se toca el de organización
-- (`corporate.sytue.eu/pages/…`): el prefijo `/pages` no cambia aquí.
--
-- La columna del identificador es `id`, NO `client_id` (ver 0001 y la nota de
-- 0094: un SQL malo aquí no falla en el despliegue, falla al levantar el IdP).
BEGIN;

-- Idempotente como 0033: añade únicamente lo que falta, así que repetir la
-- migración —o que bootstrap-id.sh del piloto la reaplique— no duplica entradas
-- ni reordena las existentes.
UPDATE oauth_clients
SET name = 'Sytue Web',
    redirect_uris = redirect_uris || ARRAY(
        SELECT u
        FROM unnest(ARRAY[
            -- La CANÓNICA: host de producto con prefijo (la forma de 0099).
            'https://web.sytue.eu/pages/auth/callback',
            -- Raíz del host, por donde entra el BFF (`/auth` → addPrefix).
            'https://web.sytue.eu/auth/callback',
            -- Dev con Traefik (deploy/compose).
            'https://web.sytue.localhost/auth/callback'
        ]) AS u
        WHERE NOT (u = ANY (redirect_uris))
    ),
    post_logout_redirect_uris = post_logout_redirect_uris || ARRAY(
        SELECT u
        FROM unnest(ARRAY[
            'https://web.sytue.eu/',
            'https://web.sytue.eu/pages/'
        ]) AS u
        WHERE NOT (u = ANY (post_logout_redirect_uris))
    ),
    updated_at = now()
WHERE id = 'sytue-pages';

COMMIT;
