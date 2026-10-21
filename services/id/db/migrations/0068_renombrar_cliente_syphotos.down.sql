-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelve el nombre de visualización del cliente OIDC de 'Sytue Photos' a 'SyPhotos'.
BEGIN;

UPDATE oauth_clients
SET name = 'SyPhotos'
WHERE id = 'sytue-syphotos';

COMMIT;
