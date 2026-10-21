-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS contacts_trash_idx;
DROP INDEX IF EXISTS contacts_favorite_idx;
ALTER TABLE contacts DROP COLUMN IF EXISTS is_favorite;
