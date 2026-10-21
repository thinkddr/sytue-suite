-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_preview_auth_len;
ALTER TABLE projects DROP COLUMN IF EXISTS preview_auth_hash;
