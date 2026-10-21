-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP TABLE IF EXISTS base_presence;
DROP INDEX IF EXISTS base_audit_op;
DROP INDEX IF EXISTS base_audit_cambios;
ALTER TABLE base_audit DROP COLUMN IF EXISTS op_id, DROP COLUMN IF EXISTS xact;
