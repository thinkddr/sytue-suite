-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira el @usuario.
-- sytue:destructive-ok deshace 0065: la columna solo la lee y la escribe el binario
-- nuevo; el anterior sigue identificando a cada persona por su correo.

DROP INDEX IF EXISTS users_username_pendiente_idx;
DROP INDEX IF EXISTS users_username_idx;
ALTER TABLE users DROP COLUMN username;
