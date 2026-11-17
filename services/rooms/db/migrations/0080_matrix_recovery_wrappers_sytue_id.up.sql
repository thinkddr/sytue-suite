-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El envoltorio `sytue-id` de la clave de recuperación (ADR 0084): cerrado con la
-- clave de desbloqueo que el navegador saca del inicio de sesión de Sytue ID. Como
-- el de contraseña, uno por cuenta de Matrix (credential_id vacío). El servicio
-- sigue guardando solo texto cifrado: la clave de desbloqueo nunca llega aquí, y
-- Sytue ID solo la guarda dentro de sobres que tampoco puede abrir.
ALTER TABLE matrix_recovery_wrappers DROP CONSTRAINT matrix_recovery_wrappers_kind_check;
ALTER TABLE matrix_recovery_wrappers DROP CONSTRAINT matrix_recovery_wrappers_check1;
ALTER TABLE matrix_recovery_wrappers
    ADD CONSTRAINT matrix_recovery_wrappers_kind_check CHECK (kind IN ('password','passkey','sytue-id'));
ALTER TABLE matrix_recovery_wrappers
    ADD CONSTRAINT matrix_recovery_wrappers_check1 CHECK ((kind = 'passkey') = (credential_id <> ''));
