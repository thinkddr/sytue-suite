-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelve a dejar el @usuario anulable y la cola del barrido. Los @usuario que
-- rellenó la subida se quedan: son válidos y la persona ya puede estar usándolos.

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_username_obligatorio;
CREATE INDEX IF NOT EXISTS users_username_pendiente_idx ON users (created_at) WHERE username IS NULL;
