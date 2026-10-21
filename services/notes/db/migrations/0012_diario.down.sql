-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP INDEX notebook_daily_owner;
ALTER TABLE notebooks DROP COLUMN daily_date;
