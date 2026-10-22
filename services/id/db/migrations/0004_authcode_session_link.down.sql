-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS refresh_tokens_session_idx;
ALTER TABLE auth_codes DROP COLUMN IF EXISTS session_id;
