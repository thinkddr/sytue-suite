-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP TABLE IF EXISTS drive_file_provenance;
ALTER TABLE files DROP CONSTRAINT IF EXISTS files_tenant_id_key;
