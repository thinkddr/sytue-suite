-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS deployments_logs_collect_idx;
ALTER TABLE deployments DROP COLUMN IF EXISTS logs_collect_claimed_at;
ALTER TABLE deployments DROP COLUMN IF EXISTS logs_collected_at;
-- Las políticas y los índices caen con la tabla.
DROP TABLE IF EXISTS runtime_logs;
