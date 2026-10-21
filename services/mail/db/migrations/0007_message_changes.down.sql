-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS messages_changes_idx;
ALTER TABLE messages DROP COLUMN IF EXISTS updated_at;
