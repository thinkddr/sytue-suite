-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira el esquema de Sytue Bases del client móvil.
-- sytue:destructive-ok quitar un redirect_uri no borra datos: la app de Bases vuelve a
-- no poder iniciar sesión en el teléfono, que es como estaba.
BEGIN;

UPDATE oauth_clients
SET redirect_uris = array_remove(redirect_uris, 'sytue-bases://auth/callback')
WHERE id = 'sytue-mobile'
  AND 'sytue-bases://auth/callback' = ANY (redirect_uris);

COMMIT;
