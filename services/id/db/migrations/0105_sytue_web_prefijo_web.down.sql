-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Camino de vuelta de la 0105: retira los callbacks con el prefijo `/web`. Los de
-- `/pages` (0102 y 0104) no se tocan: son los que anuncia la release anterior.
BEGIN;

UPDATE oauth_clients
SET redirect_uris = ARRAY(
        SELECT u
        FROM unnest(redirect_uris) WITH ORDINALITY AS t(u, n)
        WHERE u NOT IN (
            'https://web.sytue.eu/web/auth/callback',
            'https://corporate.sytue.eu/web/auth/callback'
        )
        ORDER BY n
    ),
    post_logout_redirect_uris = ARRAY(
        SELECT u
        FROM unnest(post_logout_redirect_uris) WITH ORDINALITY AS t(u, n)
        WHERE u NOT IN ('https://web.sytue.eu/web/', 'https://corporate.sytue.eu/web/')
        ORDER BY n
    ),
    updated_at = now()
WHERE id = 'sytue-pages';

COMMIT;
