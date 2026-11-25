-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Quita el lazo sesión → dispositivo de 0128. Los tokens vuelven a no poder decir
-- `device_trust=standard`.
DROP INDEX IF EXISTS sessions_device_idx;
ALTER TABLE sessions DROP COLUMN IF EXISTS device_id;
