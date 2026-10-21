-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS projects_activos_idx;
ALTER TABLE projects DROP COLUMN IF EXISTS paused;
