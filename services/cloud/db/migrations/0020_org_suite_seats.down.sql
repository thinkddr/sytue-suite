-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE organizations DROP CONSTRAINT IF EXISTS organizations_suite_seats_nonneg;
ALTER TABLE organizations DROP COLUMN IF EXISTS suite_seats;
