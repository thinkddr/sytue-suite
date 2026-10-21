-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS channel_members_favorites;
ALTER TABLE channel_members DROP COLUMN IF EXISTS muted;
ALTER TABLE channel_members DROP COLUMN IF EXISTS favorite;
