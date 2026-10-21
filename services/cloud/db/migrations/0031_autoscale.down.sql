-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_autoscale_max_range;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_autoscale_cpu_range;
ALTER TABLE projects
    DROP COLUMN IF EXISTS autoscale_max_count,
    DROP COLUMN IF EXISTS autoscale_cpu_target;
