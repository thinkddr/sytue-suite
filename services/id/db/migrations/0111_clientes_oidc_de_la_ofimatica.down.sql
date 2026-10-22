-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- sytue:destructive-ok
BEGIN;
DELETE FROM oauth_clients
WHERE id IN ('sytue-docus', 'sytue-calcus', 'sytue-diapos', 'sytue-certipdf');
COMMIT;
