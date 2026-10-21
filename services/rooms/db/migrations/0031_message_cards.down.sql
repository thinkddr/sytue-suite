-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS message_card_votes;
ALTER TABLE messages DROP COLUMN IF EXISTS card;
