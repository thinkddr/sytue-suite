-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retirar los callbacks de los hosts LEGADOS: la transición ya terminó.
--
-- `chat.*` y `meet.*` (0023, hoy Rooms) y `console.*` (0026, hoy Admin)
-- conservaron sus callbacks A PROPÓSITO: Traefik responde 301 al host canónico,
-- pero un flujo iniciado antes del renombrado podía volver al callback antiguo y
-- se quería evitar un `invalid_redirect_uri` en mitad de la transición.
--
-- Esa razón ya no aplica. El redirect_uri no lo elige el navegador sino el BFF en
-- cada petición, a partir de `SYTUE_OIDC_REDIRECT_URI` del despliegue, y las diez
-- superficies piden hoy su host canónico (verificado en producción: `/auth/login`
-- de las diez redirige a `authorize` con el callback canónico). Ningún flujo puede
-- volver al antiguo porque ninguno lo pide.
--
-- Y sí molestan: el barrido de cierre de sesión deriva a quién preguntar de los
-- redirect_uri registrados, así que cada cierre pedía tres cierres de más que solo
-- servían para un 301 hacia un host que ya estaba en la lista.
--
-- Solo `redirect_uris`, y solo en clients de primera parte. Los destinos
-- post-logout legados se quedan: no los pide nadie (el despliegue fija el sitio
-- público, 0033) y quitarlos no arregla nada.
UPDATE oauth_clients
SET redirect_uris = ARRAY(
        SELECT u FROM unnest(redirect_uris) AS u
        WHERE u !~ '^https://(chat|meet|console)\.sytue\.(eu|localhost)(/|$)'
    ),
    updated_at = now()
WHERE managed_by = 'static'
  -- Nunca dejar un client sin ningún destino de retorno: sería un
  -- `invalid_redirect_uri` en el siguiente login, y ningún host legado es el
  -- único callback de nadie.
  AND EXISTS (
        SELECT 1 FROM unnest(redirect_uris) AS u
        WHERE u !~ '^https://(chat|meet|console)\.sytue\.(eu|localhost)(/|$)'
    );
