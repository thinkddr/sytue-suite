-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira el cambio de @usuario y de móvil desde Mi cuenta. Los @usuario y los móviles
-- ya cambiados se quedan: son válidos y la persona los está usando.
-- sytue:destructive-ok deshace 0126: las tablas y la columna solo las usa el binario nuevo.
ALTER TABLE users DROP COLUMN IF EXISTS username_changed_at;
DROP TABLE IF EXISTS usernames_retirados;
DROP TABLE IF EXISTS phone_change_codes;
