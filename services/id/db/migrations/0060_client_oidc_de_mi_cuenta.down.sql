-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira el client OIDC de «Mi cuenta».
-- sytue:destructive-ok el alta de un client es reversible por definición: sin la
-- app desplegada, la fila no sirve a nadie y su borrado no toca datos de usuario
-- (las sesiones vivas se revocan solas al no poder refrescar).

DELETE FROM oauth_clients WHERE id = 'sytue-account';
