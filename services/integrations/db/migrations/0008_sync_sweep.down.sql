-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS app_instances_active_idx;
DROP POLICY IF EXISTS app_instances_maintenance ON app_instances;
