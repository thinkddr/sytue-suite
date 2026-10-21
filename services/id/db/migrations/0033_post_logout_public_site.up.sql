-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- CERRAR SESIÓN debe devolver al sitio público, no a la app que se acaba de
-- abandonar.
--
-- Hasta ahora cada client solo registraba su propio host como destino
-- post-logout, y el BFF pedía exactamente ese. El efecto: cerrabas sesión y
-- volvías a la aplicación, cuyo guard te mandaba otra vez al formulario de
-- acceso. Parecía que el logout no había funcionado —o peor, que te devolvía
-- dentro— cuando en realidad la sesión sí se había destruido.
--
-- Se AÑADE el sitio público a los destinos registrados de cada client de primera
-- parte, sin quitar los suyos: un cliente que siga pidiendo su propio host sigue
-- siendo válido. La lista es una allowlist anti open-redirect, así que sin esta
-- fila Sytue ID rechazaría el destino y caería al comportamiento por defecto.
-- Solo los clients de PRIMERA PARTE (`static`). A una app de terceros registrada
-- por DCR no se le añade un destino nuestro: sus URIs son suyas.
--
-- Idempotente: añade únicamente lo que falta, así que reejecutar la migración no
-- duplica entradas ni reordena las existentes.
UPDATE oauth_clients
SET post_logout_redirect_uris = post_logout_redirect_uris || ARRAY(
        SELECT u
        FROM unnest(ARRAY['https://www.sytue.eu/', 'https://www.sytue.localhost/']) AS u
        WHERE NOT (u = ANY (post_logout_redirect_uris))
    ),
    updated_at = now()
WHERE managed_by = 'static';
