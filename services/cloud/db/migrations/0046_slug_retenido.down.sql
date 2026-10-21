-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

DROP INDEX IF EXISTS projects_slug_retenido_idx;
ALTER TABLE projects DROP COLUMN IF EXISTS purged_at;
