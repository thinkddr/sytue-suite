-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP INDEX IF EXISTS users_supresion_pendiente_idx;
ALTER TABLE users DROP COLUMN IF EXISTS erasure_request_id;
DROP TABLE IF EXISTS dsr_request_events;
DROP FUNCTION IF EXISTS dsr_request_events_solo_anadir();
DROP TABLE IF EXISTS dsr_requests;
