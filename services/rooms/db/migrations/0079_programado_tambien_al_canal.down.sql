-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE scheduled_messages DROP COLUMN IF EXISTS also_in_channel;
