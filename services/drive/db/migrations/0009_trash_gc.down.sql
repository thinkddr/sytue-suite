-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP INDEX IF EXISTS files_gc_idx;
DROP INDEX IF EXISTS folders_gc_idx;
DROP POLICY IF EXISTS file_versions_maintenance ON file_versions;
DROP POLICY IF EXISTS quotas_maintenance ON quotas;
DROP POLICY IF EXISTS files_maintenance ON files;
DROP POLICY IF EXISTS folders_maintenance ON folders;
