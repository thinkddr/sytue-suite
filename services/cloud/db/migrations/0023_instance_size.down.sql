-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_instance_count_range;
ALTER TABLE projects
    DROP COLUMN IF EXISTS instance_size,
    DROP COLUMN IF EXISTS instance_count;
