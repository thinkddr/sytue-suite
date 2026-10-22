-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS messages_mentions_all;
ALTER TABLE messages DROP COLUMN IF EXISTS mentions_all;
