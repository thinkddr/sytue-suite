-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira el client OIDC de Sytue Cast y su esquema nativo del client móvil.
-- sytue:destructive-ok el alta de un client es reversible por definición: sin
-- Sytue Cast desplegado, la fila no sirve a nadie y su borrado no toca datos de
-- usuario (las sesiones vivas se revocan solas al no poder refrescar).
BEGIN;

DELETE FROM oauth_clients WHERE id = 'sytue-cast';

UPDATE oauth_clients
SET redirect_uris = array_remove(redirect_uris, 'sytue-cast://auth/callback')
WHERE id = 'sytue-mobile'
  AND 'sytue-cast://auth/callback' = ANY (redirect_uris);

COMMIT;
