-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte la delegación: vuelve al conjunto reader|writer. Las delegaciones
-- vigentes (permission='manage') deben degradarse a 'writer' antes de revertir.
UPDATE calendar_shares SET permission = 'writer' WHERE permission = 'manage';

ALTER TABLE calendar_shares
    DROP CONSTRAINT IF EXISTS calendar_shares_permission_check;

ALTER TABLE calendar_shares
    ADD CONSTRAINT calendar_shares_permission_check
    CHECK (permission IN ('reader', 'writer'));
