-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS users_closed_at_idx;
ALTER TABLE users
    DROP COLUMN IF EXISTS close_reason,
    DROP COLUMN IF EXISTS closed_by,
    DROP COLUMN IF EXISTS closed_at;
