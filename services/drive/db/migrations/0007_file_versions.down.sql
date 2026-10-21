-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS file_versions;
ALTER TABLE files DROP COLUMN IF EXISTS modified_by;
