-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP TRIGGER IF EXISTS base_records_contar_bajas ON base_records;
DROP TRIGGER IF EXISTS base_records_contar_altas ON base_records;
DROP FUNCTION IF EXISTS base_records_contar();
ALTER TABLE base_tables DROP COLUMN IF EXISTS record_count;
