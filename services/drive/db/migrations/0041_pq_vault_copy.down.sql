-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
ALTER TABLE pq_vaults
    DROP CONSTRAINT IF EXISTS pq_vaults_copy_whole,
    DROP COLUMN IF EXISTS copy_expires_at,
    DROP COLUMN IF EXISTS copy_started_at,
    DROP COLUMN IF EXISTS copy_epoch,
    DROP COLUMN IF EXISTS copy_kind,
    DROP COLUMN IF EXISTS copy_holder,
    DROP COLUMN IF EXISTS copy_id,
    DROP COLUMN IF EXISTS version;
