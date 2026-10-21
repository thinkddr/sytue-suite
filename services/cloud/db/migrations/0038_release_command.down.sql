-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_release_command_len;
ALTER TABLE projects DROP COLUMN IF EXISTS release_command;
