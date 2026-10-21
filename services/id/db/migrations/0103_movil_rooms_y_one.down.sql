-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira los esquemas de Sytue Rooms y del Hub del client móvil.
-- sytue:destructive-ok quitar un redirect_uri no borra datos: esas dos apps vuelven
-- a no poder iniciar sesión en el teléfono, que es como estaban.
BEGIN;

UPDATE oauth_clients
SET redirect_uris = array_remove(redirect_uris, 'sytue-rooms://auth/callback')
WHERE id = 'sytue-mobile'
  AND 'sytue-rooms://auth/callback' = ANY (redirect_uris);

UPDATE oauth_clients
SET redirect_uris = array_remove(redirect_uris, 'sytue-one://auth/callback')
WHERE id = 'sytue-mobile'
  AND 'sytue-one://auth/callback' = ANY (redirect_uris);

COMMIT;
