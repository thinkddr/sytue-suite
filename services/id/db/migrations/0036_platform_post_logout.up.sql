-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Reaplica el destino post-logout del sitio público a los clients de primera
-- parte creados DESPUÉS de la 0033.
--
-- El de platform.sytue.eu (0034) nació sin él: el BFF pide el sitio público al
-- cerrar sesión y Sytue ID lo rechazaba por no estar en su allowlist, cayendo al
-- destino por defecto del servicio. Hoy ambos coinciden, así que no se notaba;
-- el día que un despliegue cambie POST_LOGOUT_DEFAULT_URI, la administración de
-- plataforma sería la única superficie que no lo respeta.
--
-- Mismo UPDATE idempotente que la 0033: añade solo lo que falte, en cualquier
-- client `static`, así que cubre también a los que se hayan añadido entre medias.
UPDATE oauth_clients
SET post_logout_redirect_uris = post_logout_redirect_uris || ARRAY(
        SELECT u
        FROM unnest(ARRAY['https://www.sytue.eu/', 'https://www.sytue.localhost/']) AS u
        WHERE NOT (u = ANY (post_logout_redirect_uris))
    ),
    updated_at = now()
WHERE managed_by = 'static';
