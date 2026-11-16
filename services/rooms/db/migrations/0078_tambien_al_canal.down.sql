-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP INDEX IF EXISTS messages_channel_feed;
CREATE INDEX messages_channel_feed
    ON messages (tenant, channel_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL AND parent_id IS NULL;
ALTER TABLE messages DROP COLUMN IF EXISTS also_in_channel;
