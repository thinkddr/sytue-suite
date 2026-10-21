-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- «Mi cuenta» (`sytue-account`) pasa a su dirección GLOBAL, `id.<dominio>/account`
-- (decisión del dueño, 2026-09-24): la cuenta es de la persona, no de una
-- organización. El chart la anuncia así (`globalHost: id`) y su BFF salta al host
-- del `redirect_uri` antes de iniciar sesión, de modo que sin esta fila el login
-- de la cuenta moría con «redirect_uri is not registered for the client». Nada
-- lo avisa al desplegar: se ve al intentar entrar.
--
-- Se AÑADE, no se sustituye: las de `account.<dominio>` y del host de
-- organización se quedan, y no estorban —esa dirección ya solo redirige aquí—.
--
-- Y el cierre de sesión, que el chart manda a `https://www.<dominio>/`: `sytue-account`
-- nació (0060) después de la 0033 que registró ese destino en el resto, así que
-- se le añade aquí para que el logout no acabe en la pantalla de open-redirect.
--
-- Idempotente —`pg.MigrateUp` corre al arrancar cada réplica— y con la columna
-- `id`, no `client_id` (ver 0001 y la nota de 0094). Con el patrón de la 0033 y
-- no con el de la 0094: un `FROM (VALUES …)` de dos filas contra la MISMA fila de
-- destino solo aplica una de las dos.
BEGIN;

UPDATE oauth_clients
SET redirect_uris = redirect_uris || ARRAY(
        SELECT u
        FROM unnest(ARRAY['https://id.sytue.eu/account/auth/callback',
                          'https://id.sytue.localhost/account/auth/callback']) AS u
        WHERE NOT (u = ANY (redirect_uris))
    ),
    post_logout_redirect_uris = post_logout_redirect_uris || ARRAY(
        SELECT u
        FROM unnest(ARRAY['https://www.sytue.eu/', 'https://www.sytue.localhost/']) AS u
        WHERE NOT (u = ANY (post_logout_redirect_uris))
    ),
    updated_at = now()
WHERE id = 'sytue-account';

COMMIT;
