-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE events_outbox DROP COLUMN IF EXISTS locked_until;
