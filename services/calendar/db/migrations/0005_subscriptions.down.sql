-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS calendar_subscriptions;

ALTER TABLE calendars
    DROP COLUMN IF EXISTS read_only;
