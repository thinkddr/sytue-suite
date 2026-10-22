-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Deshace 0097. El cierre de sesión hacia `www` se deja: si otra migración o el
-- despliegue lo había registrado antes, quitarlo aquí rompería un logout que ya
-- funcionaba.
BEGIN;

UPDATE oauth_clients
SET redirect_uris = array_remove(array_remove(redirect_uris,
        'https://id.sytue.eu/account/auth/callback'),
        'https://id.sytue.localhost/account/auth/callback')
WHERE id = 'sytue-account';

COMMIT;
