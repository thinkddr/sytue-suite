-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP INDEX IF EXISTS group_members_user_idx;
ALTER TABLE groups DROP COLUMN IF EXISTS description;
