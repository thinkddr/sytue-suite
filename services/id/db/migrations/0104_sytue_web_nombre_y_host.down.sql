-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Camino de vuelta de la 0104: el client vuelve a llamarse «Sytue Pages» y pierde
-- los callbacks de `web.sytue.eu`. Los de `pages.sytue.eu` (0102) no se tocan: sin
-- ellos, un despliegue anterior del chart —que anunciaba ese host— dejaría de
-- poder hacer login.
BEGIN;

UPDATE oauth_clients
SET name = 'Sytue Pages',
    redirect_uris = ARRAY(
        SELECT u
        FROM unnest(redirect_uris) WITH ORDINALITY AS t(u, n)
        WHERE u NOT IN (
            'https://web.sytue.eu/pages/auth/callback',
            'https://web.sytue.eu/auth/callback',
            'https://web.sytue.localhost/auth/callback'
        )
        ORDER BY n
    ),
    post_logout_redirect_uris = ARRAY(
        SELECT u
        FROM unnest(post_logout_redirect_uris) WITH ORDINALITY AS t(u, n)
        WHERE u NOT IN ('https://web.sytue.eu/', 'https://web.sytue.eu/pages/')
        ORDER BY n
    ),
    updated_at = now()
WHERE id = 'sytue-pages';

COMMIT;
