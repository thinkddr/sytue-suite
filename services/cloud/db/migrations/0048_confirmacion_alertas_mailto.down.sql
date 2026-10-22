-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

DROP POLICY IF EXISTS alert_policies_maintenance ON alert_policies;
DROP INDEX IF EXISTS alert_policies_confirm_token_hash_idx;
ALTER TABLE alert_policies
    DROP COLUMN IF EXISTS confirm_token_hash,
    DROP COLUMN IF EXISTS confirmed_at;
