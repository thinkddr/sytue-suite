-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS message_acks;
ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_priority_valid;
ALTER TABLE messages DROP COLUMN IF EXISTS requires_ack;
ALTER TABLE messages DROP COLUMN IF EXISTS priority;
