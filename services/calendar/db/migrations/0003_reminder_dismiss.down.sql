-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS calendar_reminders_inapp_idx;
ALTER TABLE calendar_reminders DROP COLUMN IF EXISTS dismissed_at;
