-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Actualiza el nombre de visualización del cliente OIDC de 'SyPhotos' a 'Sytue Photos'.
BEGIN;

UPDATE oauth_clients
SET name = 'Sytue Photos'
WHERE id = 'sytue-syphotos';

COMMIT;
