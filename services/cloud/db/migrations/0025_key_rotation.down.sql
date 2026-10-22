-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP POLICY IF EXISTS env_vars_maintenance ON env_vars;

ALTER TABLE projects         DROP COLUMN IF EXISTS key_version;
ALTER TABLE managed_services DROP COLUMN IF EXISTS key_version;
