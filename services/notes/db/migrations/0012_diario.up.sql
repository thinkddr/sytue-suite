-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
ALTER TABLE notebooks ADD COLUMN daily_date text NOT NULL DEFAULT '';
ALTER TABLE notebooks ADD CONSTRAINT notebook_daily_date CHECK (
    daily_date = '' OR daily_date ~ '^\d{4}-\d{2}-\d{2}$'
);
CREATE UNIQUE INDEX notebook_daily_owner ON notebooks (tenant, owner_sub, daily_date)
    WHERE daily_date <> '' AND deleted_at IS NULL;
