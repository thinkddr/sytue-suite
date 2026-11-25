-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP TRIGGER IF EXISTS audit_log_solo_anadir ON audit_log;
DROP FUNCTION IF EXISTS audit_log_solo_anadir();
DROP POLICY IF EXISTS audit_log_retencion ON audit_log;
DROP POLICY IF EXISTS audit_log_retencion_ver ON audit_log;
DROP INDEX IF EXISTS audit_log_acceso_idx;
