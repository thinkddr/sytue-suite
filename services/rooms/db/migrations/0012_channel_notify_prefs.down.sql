-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS user_notify_prefs;
ALTER TABLE channel_members DROP CONSTRAINT IF EXISTS channel_members_notify_level_valid;
ALTER TABLE channel_members DROP COLUMN IF EXISTS notify_level;
