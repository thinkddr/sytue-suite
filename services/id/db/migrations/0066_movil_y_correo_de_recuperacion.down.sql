-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira el móvil, el correo de recuperación y las altas pendientes de SMS.
-- sytue:destructive-ok deshace 0066: solo el binario nuevo lee y escribe estas
-- columnas y la tabla; el anterior da de alta sin móvil y entra por correo o @usuario.

ALTER TABLE email_verification_tokens DROP COLUMN IF EXISTS purpose;
DROP TABLE IF EXISTS signup_phone_codes;
DROP INDEX IF EXISTS users_recovery_email_idx;
DROP INDEX IF EXISTS users_phone_idx;
ALTER TABLE users DROP COLUMN IF EXISTS recovery_email;
ALTER TABLE users DROP COLUMN IF EXISTS phone;
