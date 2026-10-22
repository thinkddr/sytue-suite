-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE projects
    DROP CONSTRAINT IF EXISTS projects_app_port_range,
    DROP CONSTRAINT IF EXISTS projects_health_path_abs;

ALTER TABLE projects
    DROP COLUMN IF EXISTS app_port,
    DROP COLUMN IF EXISTS health_path;
