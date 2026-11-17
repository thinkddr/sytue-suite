-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sin el tipo `sytue-id` sus filas no caben en las restricciones de la 0077.
DELETE FROM matrix_recovery_wrappers WHERE kind = 'sytue-id';
ALTER TABLE matrix_recovery_wrappers DROP CONSTRAINT matrix_recovery_wrappers_kind_check;
ALTER TABLE matrix_recovery_wrappers DROP CONSTRAINT matrix_recovery_wrappers_check1;
ALTER TABLE matrix_recovery_wrappers
    ADD CONSTRAINT matrix_recovery_wrappers_kind_check CHECK (kind IN ('password','passkey'));
ALTER TABLE matrix_recovery_wrappers
    ADD CONSTRAINT matrix_recovery_wrappers_check1 CHECK ((kind = 'password') = (credential_id = ''));
