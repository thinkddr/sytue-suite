-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS users_baja_propia_pendiente_idx;
ALTER TABLE users
    DROP COLUMN IF EXISTS purged_at,
    DROP COLUMN IF EXISTS personal_data_erased_at;
