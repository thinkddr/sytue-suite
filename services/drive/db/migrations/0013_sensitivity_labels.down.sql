-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE files
    DROP COLUMN IF EXISTS label_block_external_share,
    DROP COLUMN IF EXISTS label_rank,
    DROP COLUMN IF EXISTS label_name,
    DROP COLUMN IF EXISTS label_id;
